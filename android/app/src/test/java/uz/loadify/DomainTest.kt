package uz.loadify

import kotlinx.serialization.json.Json
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import uz.loadify.core.network.Envelope
import uz.loadify.feature.auth.domain.AuthResponse
import uz.loadify.feature.bookings.domain.Booking
import uz.loadify.feature.loads.domain.Address
import uz.loadify.feature.loads.domain.Load
import uz.loadify.feature.loads.domain.LoadFilters
import uz.loadify.feature.trips.domain.Trip
import uz.loadify.feature.trips.domain.TripStage

private val json = Json {
    ignoreUnknownKeys = true
    coerceInputValues = true
    explicitNulls = false
}

class AuthResponseTest {

    /**
     * The regression guard for a bug the web client actually shipped: the
     * tokens are nested under `tokens`, and reading them from the top level
     * yielded nothing. Refresh then "succeeded" while storing an empty token,
     * and every later request went out unauthenticated — the session died
     * silently fifteen minutes after sign-in.
     */
    @Test
    fun `reads tokens from the nested tokens object`() {
        val payload = """
            {"user":{"id":"u1","full_name":"Anvar Tursunov","permissions":["bookings.create"]},
             "tokens":{"access_token":"access","refresh_token":"refresh",
                       "expires_at":"2030-01-01T00:00:00Z"}}
        """.trimIndent()

        val auth = json.decodeFromString(AuthResponse.serializer(), payload)

        assertTrue(auth.isUsable)
        assertEquals("access", auth.tokens?.accessToken)
        assertEquals("refresh", auth.tokens?.refreshToken)
        assertEquals("Anvar Tursunov", auth.user?.fullName)
    }

    @Test
    fun `rejects a payload with the tokens spread at the top level`() {
        // The shape the OpenAPI document used to describe. Accepting this is
        // precisely the failure this guards against.
        val payload = """
            {"user":{"id":"u1"},"access_token":"access","refresh_token":"refresh"}
        """.trimIndent()

        val auth = json.decodeFromString(AuthResponse.serializer(), payload)

        assertFalse(auth.isUsable)
    }

    @Test
    fun `rejects a payload missing the user or an empty token`() {
        val noUser = """{"tokens":{"access_token":"a","refresh_token":"r"}}"""
        assertFalse(json.decodeFromString(AuthResponse.serializer(), noUser).isUsable)

        val emptyToken =
            """{"user":{"id":"u1"},"tokens":{"access_token":"","refresh_token":"r"}}"""
        assertFalse(json.decodeFromString(AuthResponse.serializer(), emptyToken).isUsable)
    }

    @Test
    fun `gates on permissions rather than role names`() {
        val payload = """
            {"user":{"id":"u1","roles":["driver"],
                     "permissions":["bookings.create","trips.read"],"driver_id":"d1"},
             "tokens":{"access_token":"a","refresh_token":"r"}}
        """.trimIndent()

        val user = json.decodeFromString(AuthResponse.serializer(), payload).user!!

        assertTrue(user.can("bookings.create"))
        assertFalse(user.can("loads.create"))
        assertTrue(user.isDriver)
    }

    @Test
    fun `an unknown field does not break parsing`() {
        // The server adds fields ahead of the client; an old install must keep
        // working the moment one ships.
        val payload = """
            {"user":{"id":"u1","loyalty_tier":"gold"},
             "tokens":{"access_token":"a","refresh_token":"r"},"issued_by":"auth-2"}
        """.trimIndent()

        assertTrue(json.decodeFromString(AuthResponse.serializer(), payload).isUsable)
    }
}

class TripStageTest {

    @Test
    fun `follows the backend state machine in order`() {
        assertEquals(TripStage.EN_ROUTE_TO_PICKUP, TripStage.CREATED.next)
        assertEquals(TripStage.AT_PICKUP, TripStage.EN_ROUTE_TO_PICKUP.next)
        assertEquals(TripStage.LOADED, TripStage.AT_PICKUP.next)
        assertEquals(TripStage.IN_TRANSIT, TripStage.LOADED.next)
        assertEquals(TripStage.AT_DELIVERY, TripStage.IN_TRANSIT.next)
        assertEquals(TripStage.DELIVERED, TripStage.AT_DELIVERY.next)
    }

    @Test
    fun `offers no next stage once delivered`() {
        // Only the shipper closes a trip. Offering the driver a button here
        // would produce a request the server is bound to reject.
        assertNull(TripStage.DELIVERED.next)
        assertNull(TripStage.COMPLETED.next)
        assertNull(TripStage.CANCELLED.next)
    }

    @Test
    fun `reports tracking only while the truck is actually moving`() {
        fun at(stage: TripStage) = Trip(status = stage.wire)

        assertFalse(at(TripStage.CREATED).isTracking)
        assertTrue(at(TripStage.EN_ROUTE_TO_PICKUP).isTracking)
        assertTrue(at(TripStage.IN_TRANSIT).isTracking)
        assertFalse(at(TripStage.DELIVERED).isTracking)
        assertFalse(at(TripStage.COMPLETED).isTracking)
    }

    @Test
    fun `an unknown wire value degrades instead of throwing`() {
        // A server that adds a stage must not crash an older app on the road.
        assertEquals(TripStage.CREATED, TripStage.parse("teleported"))
        assertEquals(TripStage.CREATED, TripStage.parse(null))
    }
}

class BookingTest {

    private fun booking(initiatedBy: String, status: String) =
        Booking(id = "b", initiatedBy = initiatedBy, status = status)

    @Test
    fun `only the side that did not initiate can respond`() {
        val fromDriver = booking("driver", "pending")
        assertFalse(fromDriver.canRespond(asDriver = true))
        assertTrue(fromDriver.canRespond(asDriver = false))
        assertTrue(fromDriver.canWithdraw(asDriver = true))

        val fromCompany = booking("company", "pending")
        assertTrue(fromCompany.canRespond(asDriver = true))
        assertFalse(fromCompany.canRespond(asDriver = false))
    }

    @Test
    fun `a settled booking offers neither action`() {
        val accepted = booking("driver", "accepted")
        assertFalse(accepted.canRespond(asDriver = false))
        assertFalse(accepted.canWithdraw(asDriver = true))
    }

    @Test
    fun `price prefers the agreed figure over the offer`() {
        val offered = Booking(offeredPrice = 100_00, agreedPrice = null)
        assertEquals(100_00L, offered.price)

        // Showing the offer once a different figure was agreed would be a lie.
        val agreed = Booking(offeredPrice = 100_00, agreedPrice = 90_00)
        assertEquals(90_00L, agreed.price)
    }
}

class LoadTest {

    @Test
    fun `address falls back through labels rather than showing nothing`() {
        val full = Address(regionName = "Tashkent City", cityName = "Tashkent", line1 = "Line")
        assertEquals("Tashkent City", full.shortLabel)

        val noRegion = Address(cityName = "Bukhara", line1 = "Line")
        assertEquals("Bukhara", noRegion.shortLabel)

        assertEquals("—", Address().shortLabel)
    }

    @Test
    fun `money survives a large minor-unit value`() {
        // 620 000 so'm is 62 000 000 tiyin, which overflows a 32-bit int.
        val payload = """{"id":"l1","price_amount":62000000,"weight_kg":20000}"""
        val load = json.decodeFromString(Load.serializer(), payload)

        assertEquals(62_000_000L, load.priceAmount)
        assertEquals(20_000L, load.weightKg)
    }

    @Test
    fun `a list envelope carries its page metadata`() {
        val payload = """
            {"success":true,"data":[{"id":"l1"}],
             "meta":{"page":2,"per_page":20,"total":41,"total_pages":3}}
        """.trimIndent()

        val envelope = json.decodeFromString(
            Envelope.serializer(kotlinx.serialization.builtins.ListSerializer(Load.serializer())),
            payload,
        )

        assertEquals(1, envelope.data?.size)
        assertEquals(41, envelope.meta?.total)
        assertTrue(envelope.meta!!.hasMore)
    }
}

class LoadFiltersTest {

    @Test
    fun `empty filters send no filter keys`() {
        val query = LoadFilters().toQuery()

        assertFalse(query.containsKey("pickup_region_id"))
        assertFalse(query.containsKey("fits_my_vehicle"))
        assertEquals("1", query["page"])
    }

    @Test
    fun `set filters appear in the query`() {
        val query = LoadFilters(
            pickupRegionId = "r1",
            vehicleType = "tent",
            minWeightKg = 5_000,
            fitsMyVehicle = true,
        ).toQuery()

        assertEquals("r1", query["pickup_region_id"])
        assertEquals("tent", query["vehicle_type"])
        assertEquals("5000", query["min_weight_kg"])
        assertEquals("true", query["fits_my_vehicle"])
    }

    @Test
    fun `isEmpty ignores the page number`() {
        // Paging is not a filter; treating it as one would light up the badge
        // on the filter button just because the driver scrolled.
        assertTrue(LoadFilters(page = 3).isEmpty)
        assertFalse(LoadFilters(vehicleType = "tent").isEmpty)
    }
}
