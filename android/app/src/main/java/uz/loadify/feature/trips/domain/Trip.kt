package uz.loadify.feature.trips.domain

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import uz.loadify.R
import uz.loadify.feature.loads.domain.GeoPoint
import uz.loadify.feature.loads.domain.LoadCompany

@Serializable
data class LoadSummary(
    val id: String = "",
    @SerialName("reference_no") val referenceNo: String = "",
    val title: String = "",
    val status: String = "",
    @SerialName("weight_kg") val weightKg: Long = 0,
    @SerialName("pickup_formatted") val pickupFormatted: String = "",
    @SerialName("delivery_formatted") val deliveryFormatted: String = "",
    @SerialName("price_amount") val priceAmount: Long = 0,
    @SerialName("price_currency") val priceCurrency: String = "UZS",
    @SerialName("distance_km") val distanceKm: Double? = null,
)

@Serializable
data class Party(
    @SerialName("user_id") val userId: String = "",
    @SerialName("full_name") val fullName: String = "",
    @SerialName("avatar_url") val avatarUrl: String? = null,
    val rating: Double = 0.0,
    val trips: Int = 0,
    val phone: String? = null,
)

@Serializable
data class Vehicle(
    val id: String = "",
    val type: String = "",
    val make: String = "",
    val model: String = "",
    @SerialName("plate_number") val plateNumber: String = "",
    @SerialName("capacity_kg") val capacityKg: Long = 0,
    @SerialName("volume_m3") val volumeM3: Double? = null,
) {
    val label: String get() = "$make $model".trim()
}

/**
 * The trip state machine, in the order the backend enforces.
 *
 * This is the client's mirror of the server's transitions, not an independent
 * opinion: the button offers [next], and the server remains the authority that
 * rejects an out-of-order move.
 */
enum class TripStage(val wire: String, val labelRes: Int) {
    CREATED("created", R.string.stage_created),
    EN_ROUTE_TO_PICKUP("en_route_to_pickup", R.string.stage_en_route_to_pickup),
    AT_PICKUP("at_pickup", R.string.stage_at_pickup),
    LOADED("loaded", R.string.stage_loaded),
    IN_TRANSIT("in_transit", R.string.stage_in_transit),
    AT_DELIVERY("at_delivery", R.string.stage_at_delivery),
    DELIVERED("delivered", R.string.stage_delivered),
    COMPLETED("completed", R.string.stage_completed),
    CANCELLED("cancelled", R.string.stage_cancelled),
    DISPUTED("disputed", R.string.stage_disputed);

    /**
     * What the driver's advance button moves to, or null at the end of the
     * driver's part of the flow — only the shipper can close a trip.
     */
    val next: TripStage?
        get() {
            val i = DRIVER_FLOW.indexOf(this)
            return if (i < 0 || i + 1 >= DRIVER_FLOW.size) null else DRIVER_FLOW[i + 1]
        }

    val isTerminal: Boolean get() = this == COMPLETED || this == CANCELLED || this == DISPUTED

    companion object {
        /**
         * The stages a driver walks through, in order. `completed` is excluded
         * because only the shipper confirms, and the terminal states are not
         * part of the happy path.
         */
        val DRIVER_FLOW = listOf(
            CREATED,
            EN_ROUTE_TO_PICKUP,
            AT_PICKUP,
            LOADED,
            IN_TRANSIT,
            AT_DELIVERY,
            DELIVERED,
        )

        /** An unknown stage degrades rather than crashing an older app. */
        fun parse(value: String?): TripStage =
            entries.firstOrNull { it.wire == value } ?: CREATED
    }
}

@Serializable
data class Trip(
    val id: String = "",
    @SerialName("trip_number") val tripNumber: String = "",
    @SerialName("booking_id") val bookingId: String = "",
    @SerialName("load_id") val loadId: String = "",
    val load: LoadSummary? = null,
    val driver: Party? = null,
    val company: LoadCompany? = null,
    val vehicle: Vehicle? = null,
    val status: String = "created",
    @SerialName("agreed_price") val agreedPrice: Long = 0,
    @SerialName("commission_amount") val commissionAmount: Long = 0,
    @SerialName("driver_payout") val driverPayout: Long = 0,
    val currency: String = "UZS",
    @SerialName("current_location") val currentLocation: GeoPoint? = null,
    @SerialName("eta_at") val etaAt: String? = null,
    @SerialName("started_at") val startedAt: String? = null,
    @SerialName("delivered_at") val deliveredAt: String? = null,
    @SerialName("can_rate") val canRate: Boolean = false,
) {
    val stage: TripStage get() = TripStage.parse(status)

    /**
     * A trip is worth reporting position for only while it is actually moving.
     * Pinging before pickup or after delivery burns battery for nothing.
     */
    val isTracking: Boolean
        get() = stage == TripStage.EN_ROUTE_TO_PICKUP ||
            stage == TripStage.LOADED ||
            stage == TripStage.IN_TRANSIT ||
            stage == TripStage.AT_DELIVERY
}
