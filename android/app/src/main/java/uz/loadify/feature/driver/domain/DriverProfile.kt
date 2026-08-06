package uz.loadify.feature.driver.domain

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import uz.loadify.R
import uz.loadify.feature.loads.domain.GeoPoint
import uz.loadify.feature.trips.domain.Vehicle

/**
 * Whether the driver is taking work right now.
 *
 * `busy` is set by the server while a trip is running, not by the driver, so
 * the toggle in the UI only ever moves between online and offline.
 */
enum class DriverStatus(val wire: String, val labelRes: Int) {
    OFFLINE("offline", R.string.profile_offline),
    ONLINE("online", R.string.profile_online),
    BUSY("busy", R.string.profile_busy);

    val isAvailable: Boolean get() = this == ONLINE

    companion object {
        fun parse(value: String?): DriverStatus =
            entries.firstOrNull { it.wire == value } ?: OFFLINE
    }
}

@Serializable
data class DriverProfile(
    val id: String = "",
    @SerialName("user_id") val userId: String = "",
    @SerialName("full_name") val fullName: String = "",
    val phone: String? = null,
    @SerialName("avatar_url") val avatarUrl: String? = null,
    @SerialName("experience_years") val experienceYears: Int = 0,
    val status: String = "offline",
    @SerialName("verification_status") val verificationStatus: String = "pending",
    val rating: Double = 0.0,
    @SerialName("rating_count") val ratingCount: Int = 0,
    @SerialName("completed_trips") val completedTrips: Int = 0,
    @SerialName("acceptance_rate") val acceptanceRate: Double = 0.0,
    @SerialName("current_location") val currentLocation: GeoPoint? = null,
    val vehicles: List<Vehicle> = emptyList(),
) {
    val driverStatus: DriverStatus get() = DriverStatus.parse(status)
    val isVerified: Boolean get() = verificationStatus == "verified"
}

@Serializable
data class Wallet(
    val id: String = "",
    val currency: String = "UZS",
    val balance: Long = 0,
    @SerialName("held_balance") val heldBalance: Long = 0,
    val available: Long = 0,
    val status: String = "active",
)
