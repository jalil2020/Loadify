package uz.loadify.feature.bookings.domain

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import uz.loadify.feature.loads.domain.LoadCompany
import uz.loadify.feature.trips.domain.LoadSummary
import uz.loadify.feature.trips.domain.Party
import uz.loadify.feature.trips.domain.Vehicle

/**
 * A booking, from either side.
 *
 * The same object serves the driver and the shipper; which counterparty is
 * interesting depends on who is looking, which is why the accessors take the
 * viewer rather than the screen picking a field.
 */
@Serializable
data class Booking(
    val id: String = "",
    @SerialName("load_id") val loadId: String = "",
    val load: LoadSummary? = null,
    @SerialName("driver_id") val driverId: String = "",
    val driver: Party? = null,
    @SerialName("company_id") val companyId: String = "",
    val company: LoadCompany? = null,
    val vehicle: Vehicle? = null,
    @SerialName("initiated_by") val initiatedBy: String = "driver",
    val status: String = "",
    @SerialName("offered_price") val offeredPrice: Long? = null,
    @SerialName("agreed_price") val agreedPrice: Long? = null,
    val currency: String = "UZS",
    val message: String? = null,
    @SerialName("response_message") val responseMessage: String? = null,
    @SerialName("expires_at") val expiresAt: String? = null,
    @SerialName("trip_id") val tripId: String? = null,
    @SerialName("created_at") val createdAt: String = "",
) {
    /**
     * What the driver will actually be paid, if agreed; otherwise what was
     * offered. Showing the offer as though it were settled would be a lie.
     */
    val price: Long get() = agreedPrice ?: offeredPrice ?: 0

    val isPending: Boolean get() = status == "pending" || status == "offered"

    /**
     * Only the side that did not initiate can accept or reject; the initiator
     * withdraws instead.
     */
    fun canRespond(asDriver: Boolean): Boolean =
        isPending && if (asDriver) initiatedBy == "company" else initiatedBy == "driver"

    fun canWithdraw(asDriver: Boolean): Boolean =
        isPending && if (asDriver) initiatedBy == "driver" else initiatedBy == "company"

    fun counterpartyName(asDriver: Boolean): String =
        (if (asDriver) company?.brandName else driver?.fullName) ?: "—"

    fun counterpartyPhone(asDriver: Boolean): String? =
        if (asDriver) company?.phone else driver?.phone
}
