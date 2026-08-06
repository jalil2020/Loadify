package uz.loadify.core.format

import android.text.format.DateUtils
import java.text.NumberFormat
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle
import java.util.Locale
import kotlin.math.roundToLong

/**
 * Locale-aware rendering.
 *
 * Android ships full ICU data on device, so a locale that resolves here has
 * real data — unlike a browser, where uz can be reported as supported and still
 * render root-locale output. No probing is needed.
 */
class Fmt(tag: String) {

    private val locale: Locale = Locale.forLanguageTag(tag)

    /**
     * Money arrives as a Long in minor units. UZS is shown without decimals
     * because tiyin are not used in practice, and two trailing zeros on every
     * price is noise on a phone-sized screen.
     */
    fun money(minor: Long, currency: String = "UZS"): String {
        val decimals = if (currency == "UZS") 0 else 2
        val f = NumberFormat.getNumberInstance(locale).apply {
            minimumFractionDigits = decimals
            maximumFractionDigits = decimals
        }
        return "${f.format(minor / 100.0)} $currency"
    }

    /**
     * A derived rate arrives fractional. Tiyin below the unit are noise on a
     * price-per-km, so it rounds rather than widening every money field to a
     * Double and losing the exactness that matters on an actual amount.
     */
    fun money(minor: Double, currency: String = "UZS"): String =
        money(minor.roundToLong(), currency)

    fun number(value: Number): String = NumberFormat.getNumberInstance(locale).format(value)

    /** Kilograms, switching to tonnes once that reads better. */
    fun weight(kg: Long): String = if (kg >= 1000) {
        val f = NumberFormat.getNumberInstance(locale).apply { maximumFractionDigits = 1 }
        "${f.format(kg / 1000.0)} t"
    } else {
        "${number(kg)} kg"
    }

    fun distance(km: Double?): String {
        if (km == null) return EM_DASH
        val f = NumberFormat.getNumberInstance(locale).apply { maximumFractionDigits = 0 }
        return "${f.format(km)} km"
    }

    fun date(iso: String?): String {
        val instant = parse(iso) ?: return EM_DASH
        return DateTimeFormatter.ofLocalizedDate(FormatStyle.MEDIUM)
            .withLocale(locale)
            .format(instant.atZone(ZoneId.systemDefault()))
    }

    fun dateTime(iso: String?): String {
        val instant = parse(iso) ?: return EM_DASH
        return DateTimeFormatter.ofLocalizedDateTime(FormatStyle.SHORT)
            .withLocale(locale)
            .format(instant.atZone(ZoneId.systemDefault()))
    }

    /**
     * Relative time, which is what a load board actually needs: "3 soat oldin"
     * tells a driver more than a timestamp does.
     *
     * DateUtils is the platform's own phrasing, so it is translated for every
     * locale Android ships — including the ones this app does not itself carry.
     */
    fun relative(iso: String?): String {
        val instant = parse(iso) ?: return EM_DASH
        return DateUtils.getRelativeTimeSpanString(
            instant.toEpochMilli(),
            System.currentTimeMillis(),
            DateUtils.MINUTE_IN_MILLIS,
        ).toString()
    }

    private fun parse(iso: String?): Instant? {
        if (iso.isNullOrEmpty()) return null
        return try {
            // The API sends RFC 3339 with an offset; parsing to Instant
            // normalises it, so later comparisons against now are not subtly
            // wrong by the server's timezone.
            java.time.OffsetDateTime.parse(iso).toInstant()
        } catch (_: java.time.format.DateTimeParseException) {
            null
        }
    }

    companion object {
        const val EM_DASH = "—"

        private val UZ_PHONE = Regex("""^\+998(\d{2})(\d{3})(\d{2})(\d{2})$""")

        /** +998901234567 -> +998 90 123 45 67 */
        fun phone(value: String?): String {
            if (value.isNullOrEmpty()) return EM_DASH
            val m = UZ_PHONE.find(value) ?: return value
            val (a, b, c, d) = m.destructured
            return "+998 $a $b $c $d"
        }
    }
}
