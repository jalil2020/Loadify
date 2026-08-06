package uz.loadify.core.network

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import retrofit2.Response
import retrofit2.http.Body
import retrofit2.http.GET
import retrofit2.http.POST
import retrofit2.http.Path
import retrofit2.http.Query
import retrofit2.http.Tag
import retrofit2.http.QueryMap
import uz.loadify.feature.auth.domain.AuthResponse
import uz.loadify.feature.auth.domain.CurrentUser
import uz.loadify.feature.bookings.domain.Booking
import uz.loadify.feature.driver.domain.DriverProfile
import uz.loadify.feature.driver.domain.Wallet
import uz.loadify.feature.loads.domain.Load
import uz.loadify.feature.loads.domain.Region
import uz.loadify.feature.loads.domain.VehicleTypeOption
import uz.loadify.feature.trips.domain.Trip

@Serializable
data class LoginRequest(val phone: String, val password: String)

@Serializable
data class RefreshRequest(@SerialName("refresh_token") val refreshToken: String)

@Serializable
data class RespondRequest(val accept: Boolean, val message: String? = null)

@Serializable
data class AdvanceRequest(val status: String)

@Serializable
data class StatusRequest(val status: String)

@Serializable
data class LocationRequest(
    val lat: Double,
    val lng: Double,
    @SerialName("speed_kph") val speedKph: Double? = null,
    @SerialName("heading_deg") val headingDeg: Double? = null,
)

/**
 * Every endpoint returns `Response<Envelope<T>>` rather than a bare `T`.
 *
 * Retrofit would otherwise throw on any non-2xx before the error envelope could
 * be read, and the backend's machine code is the whole point of that envelope —
 * VEHICLE_UNSUITABLE has to reach the driver, not become a generic HTTP 422.
 */
interface LoadifyApi {

    // ── auth ────────────────────────────────────────────────────────────

    @POST("auth/login")
    suspend fun login(
        @Body body: LoginRequest,
        @Tag anonymous: Anonymous? = Anonymous,
    ): Response<Envelope<AuthResponse>>

    @POST("auth/logout")
    suspend fun logout(): Response<Envelope<Unit>>

    @GET("auth/me")
    suspend fun me(): Response<Envelope<CurrentUser>>

    // ── reference ───────────────────────────────────────────────────────

    @GET("reference/regions")
    suspend fun regions(
        @Tag anonymous: Anonymous? = Anonymous,
    ): Response<Envelope<List<Region>>>

    @GET("reference/vehicle-types")
    suspend fun vehicleTypes(
        @Tag anonymous: Anonymous? = Anonymous,
    ): Response<Envelope<List<VehicleTypeOption>>>

    // ── loads ───────────────────────────────────────────────────────────

    @GET("loads")
    suspend fun searchLoads(
        @QueryMap query: Map<String, String>,
        @Tag anonymous: Anonymous?,
    ): Response<Envelope<List<Load>>>

    @GET("loads/{id}")
    suspend fun load(
        @Path("id") id: String,
        @Tag anonymous: Anonymous?,
    ): Response<Envelope<Load>>

    @POST("loads/{id}/reserve")
    suspend fun reserve(@Path("id") id: String): Response<Envelope<Booking>>

    // ── bookings ────────────────────────────────────────────────────────

    @GET("bookings")
    suspend fun bookings(
        @Query("status") status: String? = null,
        @Query("page") page: Int = 1,
        @Query("per_page") perPage: Int = 20,
    ): Response<Envelope<List<Booking>>>

    @POST("bookings/{id}/respond")
    suspend fun respond(
        @Path("id") id: String,
        @Body body: RespondRequest,
    ): Response<Envelope<Booking>>

    @POST("bookings/{id}/withdraw")
    suspend fun withdraw(@Path("id") id: String): Response<Envelope<Unit>>

    // ── trips ───────────────────────────────────────────────────────────

    @GET("trips")
    suspend fun trips(
        @Query("status") status: String,
        @Query("page") page: Int = 1,
        @Query("per_page") perPage: Int = 20,
    ): Response<Envelope<List<Trip>>>

    @POST("trips/{id}/advance")
    suspend fun advance(
        @Path("id") id: String,
        @Body body: AdvanceRequest,
    ): Response<Envelope<Trip>>

    // ── driver ──────────────────────────────────────────────────────────

    @GET("drivers/me")
    suspend fun driverProfile(): Response<Envelope<DriverProfile>>

    @POST("drivers/me/status")
    suspend fun setDriverStatus(@Body body: StatusRequest): Response<Envelope<DriverProfile>>

    @POST("drivers/me/location")
    suspend fun reportLocation(@Body body: LocationRequest): Response<Envelope<Unit>>

    @GET("wallet")
    suspend fun wallet(): Response<Envelope<Wallet>>
}
