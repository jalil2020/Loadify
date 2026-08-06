package uz.loadify.feature.loads.domain

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

@Serializable
data class GeoPoint(val lat: Double = 0.0, val lng: Double = 0.0)

@Serializable
data class Address(
    val id: String = "",
    @SerialName("region_name") val regionName: String? = null,
    @SerialName("city_name") val cityName: String? = null,
    val line1: String = "",
    val formatted: String = "",
    val location: GeoPoint? = null,
    @SerialName("contact_name") val contactName: String? = null,
    @SerialName("contact_phone") val contactPhone: String? = null,
) {
    /**
     * What a driver actually scans for on a board: the region, falling back
     * through progressively narrower labels rather than showing nothing.
     */
    val shortLabel: String
        get() = listOf(regionName, cityName, line1, formatted)
            .firstOrNull { !it.isNullOrEmpty() } ?: "—"
}

@Serializable
data class LoadCompany(
    val id: String = "",
    @SerialName("brand_name") val brandName: String = "",
    @SerialName("logo_url") val logoUrl: String? = null,
    val rating: Double = 0.0,
    @SerialName("completed_trips") val completedTrips: Int = 0,
    @SerialName("is_verified") val isVerified: Boolean = false,
    /**
     * Present only once the viewer has earned contact access — that is, once a
     * live booking exists. Null here is the masked state, not missing data.
     */
    val phone: String? = null,
)

@Serializable
data class BookingSummary(
    val id: String = "",
    val status: String = "",
)

/**
 * A load on the board.
 *
 * Money is a Long in minor units (tiyin) with an explicit currency, never a
 * Double — 62_000_000 is 620 000 so'm. Anything that renders it goes through
 * the formatter.
 */
@Serializable
data class Load(
    val id: String = "",
    @SerialName("reference_no") val referenceNo: String = "",
    val title: String = "",
    val description: String? = null,
    @SerialName("weight_kg") val weightKg: Long = 0,
    @SerialName("pickup_address") val pickupAddress: Address? = null,
    @SerialName("delivery_address") val deliveryAddress: Address? = null,
    @SerialName("pickup_from") val pickupFrom: String = "",
    @SerialName("distance_km") val distanceKm: Double? = null,
    @SerialName("required_vehicle_type") val requiredVehicleType: String? = null,
    @SerialName("requires_adr") val requiresAdr: Boolean = false,
    @SerialName("requires_refrigeration") val requiresRefrigeration: Boolean = false,
    @SerialName("price_amount") val priceAmount: Long = 0,
    @SerialName("price_currency") val priceCurrency: String = "UZS",
    /**
     * A derived rate — price divided by distance — so unlike every other money
     * field this one genuinely arrives fractional (84334.235… tiyin per km).
     * Declaring it Long makes the whole list fail to decode.
     */
    @SerialName("price_per_km") val pricePerKm: Double? = null,
    val status: String = "",
    @SerialName("published_at") val publishedAt: String? = null,
    @SerialName("views_count") val viewsCount: Int = 0,
    @SerialName("is_urgent") val isUrgent: Boolean = false,
    val company: LoadCompany? = null,
    @SerialName("my_booking") val myBooking: BookingSummary? = null,
) {
    val isBooked: Boolean get() = myBooking != null
}

/**
 * The board's filter state.
 *
 * A value type, so a change is a new object and the query re-runs on identity
 * rather than on mutation.
 */
data class LoadFilters(
    val pickupRegionId: String? = null,
    val deliveryRegionId: String? = null,
    val vehicleType: String? = null,
    val minWeightKg: Long? = null,
    val maxWeightKg: Long? = null,
    val fitsMyVehicle: Boolean = false,
    val page: Int = 1,
) {
    val isEmpty: Boolean
        get() = pickupRegionId == null &&
            deliveryRegionId == null &&
            vehicleType == null &&
            minWeightKg == null &&
            maxWeightKg == null &&
            !fitsMyVehicle

    /** Drops nulls; the query builder never sends an empty filter key. */
    fun toQuery(): Map<String, String> = buildMap {
        pickupRegionId?.let { put("pickup_region_id", it) }
        deliveryRegionId?.let { put("delivery_region_id", it) }
        vehicleType?.let { put("vehicle_type", it) }
        minWeightKg?.let { put("min_weight_kg", it.toString()) }
        maxWeightKg?.let { put("max_weight_kg", it.toString()) }
        if (fitsMyVehicle) put("fits_my_vehicle", "true")
        put("page", page.toString())
        put("per_page", PER_PAGE.toString())
    }

    private companion object {
        const val PER_PAGE = 20
    }
}

/**
 * A reference vocabulary entry — region or vehicle type.
 *
 * The server returns these already localised for the request's Accept-Language,
 * so the client never translates them itself.
 */
@Serializable
data class Region(
    val id: String = "",
    val code: String = "",
    val name: String = "",
)

@Serializable
data class VehicleTypeOption(
    val code: String = "",
    val name: String = "",
)
