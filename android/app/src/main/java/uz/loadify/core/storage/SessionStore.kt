package uz.loadify.core.storage

import android.content.Context
import android.content.SharedPreferences
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.serialization.json.Json
import uz.loadify.feature.auth.domain.CurrentUser

/** The signed-in session. */
data class Session(
    val accessToken: String,
    val refreshToken: String,
    val expiresAt: String,
    val user: CurrentUser,
)

/**
 * Persists the session across launches.
 *
 * All of it lives in EncryptedSharedPreferences rather than only the refresh
 * token: the access token is a bearer credential too, and the cached principal
 * carries the driver's phone number. The keystore-backed read is fast enough
 * that a cold start still shows their name before the network answers.
 *
 * Reads are synchronous by design — the OkHttp Authenticator runs on a blocking
 * network thread and cannot suspend to fetch a token.
 */
class SessionStore(
    context: Context,
    private val json: Json,
) : SessionHolder {
    private val prefs: SharedPreferences = run {
        val key = MasterKey.Builder(context)
            .setKeyScheme(MasterKey.KeyScheme.AES256_GCM)
            .build()
        EncryptedSharedPreferences.create(
            context,
            PREFS_NAME,
            key,
            EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
            EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM,
        )
    }

    private val _session = MutableStateFlow(readFromDisk())

    /** Emits on sign-in, refresh and sign-out so the UI can react. */
    val session: StateFlow<Session?> = _session.asStateFlow()

    override val current: Session? get() = _session.value

    private val _locale = MutableStateFlow(prefs.getString(KEY_LOCALE, null) ?: DEFAULT_LOCALE)
    val locale: StateFlow<String> = _locale.asStateFlow()

    override val currentLocale: String get() = _locale.value

    private fun readFromDisk(): Session? {
        val access = prefs.getString(KEY_ACCESS, null) ?: return null
        val refresh = prefs.getString(KEY_REFRESH, null) ?: return null
        val rawUser = prefs.getString(KEY_USER, null) ?: return null
        return try {
            Session(
                accessToken = access,
                refreshToken = refresh,
                expiresAt = prefs.getString(KEY_EXPIRES, null).orEmpty(),
                user = json.decodeFromString(CurrentUser.serializer(), rawUser),
            )
        } catch (_: IllegalArgumentException) {
            // A stored session we cannot parse is worse than none: it would send
            // a half-built principal into permission checks.
            clear()
            null
        }
    }

    override fun save(session: Session) {
        prefs.edit()
            .putString(KEY_ACCESS, session.accessToken)
            .putString(KEY_REFRESH, session.refreshToken)
            .putString(KEY_EXPIRES, session.expiresAt)
            .putString(KEY_USER, json.encodeToString(CurrentUser.serializer(), session.user))
            .apply()
        _session.value = session
    }

    override fun clear() {
        prefs.edit()
            .remove(KEY_ACCESS)
            .remove(KEY_REFRESH)
            .remove(KEY_EXPIRES)
            .remove(KEY_USER)
            .apply()
        _session.value = null
    }

    fun setLocale(locale: String) {
        prefs.edit().putString(KEY_LOCALE, locale).apply()
        _locale.value = locale
    }

    private companion object {
        const val PREFS_NAME = "loadify.session"
        const val KEY_ACCESS = "access_token"
        const val KEY_REFRESH = "refresh_token"
        const val KEY_EXPIRES = "expires_at"
        const val KEY_USER = "user"
        const val KEY_LOCALE = "locale"
        const val DEFAULT_LOCALE = "uz"
    }
}
