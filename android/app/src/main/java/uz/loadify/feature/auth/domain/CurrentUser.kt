package uz.loadify.feature.auth.domain

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/** The signed-in principal. */
@Serializable
data class CurrentUser(
    val id: String = "",
    val phone: String = "",
    val email: String? = null,
    @SerialName("first_name") val firstName: String = "",
    @SerialName("last_name") val lastName: String = "",
    @SerialName("full_name") val fullName: String = "",
    @SerialName("avatar_url") val avatarUrl: String? = null,
    val locale: String = "uz",
    val status: String = "active",
    @SerialName("phone_verified") val phoneVerified: Boolean = false,
    val roles: List<String> = emptyList(),
    /**
     * The flattened effective set across every role. Gate UI on a permission,
     * never on a role name — adding a role server-side would otherwise require
     * an app release, which on mobile means waiting on a store review.
     */
    val permissions: List<String> = emptyList(),
    @SerialName("driver_id") val driverId: String? = null,
    @SerialName("company_id") val companyId: String? = null,
    @SerialName("company_role") val companyRole: String? = null,
) {
    fun can(permission: String): Boolean = permission in permissions

    val isDriver: Boolean get() = !driverId.isNullOrEmpty()

    val initials: String
        get() = fullName.split(Regex("\\s+"))
            .filter { it.isNotEmpty() }
            .take(2)
            .joinToString("") { it.first().uppercase() }
            .ifEmpty { "?" }
}

@Serializable
data class TokenPair(
    @SerialName("access_token") val accessToken: String = "",
    @SerialName("refresh_token") val refreshToken: String = "",
    @SerialName("token_type") val tokenType: String = "Bearer",
    @SerialName("expires_in") val expiresIn: Int = 0,
    @SerialName("expires_at") val expiresAt: String = "",
)

/**
 * What verify-otp, login and refresh all return.
 *
 * The tokens are nested under `tokens`, not spread at the top level. Reading
 * `data.access_token` yields nothing, which is then sent as no Authorization
 * header at all — the app looks broken rather than signed out. [isUsable] is
 * the guard: a payload that does not carry both tokens is rejected outright
 * rather than stored as a half-built session.
 */
@Serializable
data class AuthResponse(
    val user: CurrentUser? = null,
    val tokens: TokenPair? = null,
) {
    val isUsable: Boolean
        get() = user != null &&
            !tokens?.accessToken.isNullOrEmpty() &&
            !tokens?.refreshToken.isNullOrEmpty()
}
