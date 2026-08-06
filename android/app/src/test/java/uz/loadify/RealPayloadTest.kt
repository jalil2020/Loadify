package uz.loadify

import kotlinx.serialization.builtins.ListSerializer
import kotlinx.serialization.json.Json
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test
import uz.loadify.core.network.Envelope
import uz.loadify.feature.loads.domain.Load
import uz.loadify.feature.loads.domain.Region
import uz.loadify.feature.loads.domain.VehicleTypeOption

/**
 * Decodes responses captured verbatim from the running API.
 *
 * The hand-written JSON in DomainTest proves the models handle the shapes we
 * *expect*; these prove they handle the shapes the server actually sends. That
 * distinction is not academic — every field in this fixture looked obviously
 * right until `price_per_km` turned out to be a derived rate carrying a
 * fraction, which a Long field refuses to decode. The board rendered an error
 * screen and nothing said why.
 *
 * Re-capture with:
 *   curl -s "$API/loads?per_page=2" > app/src/test/resources/fixtures/loads.json
 */
class RealPayloadTest {

    private val json = Json {
        // Identical to the app's configuration in NetworkModule; a laxer parser
        // here would let a mismatch through that production would reject.
        ignoreUnknownKeys = true
        coerceInputValues = true
        explicitNulls = false
    }

    private fun fixture(name: String): String =
        checkNotNull(javaClass.getResourceAsStream("/fixtures/$name")) {
            "missing fixture $name"
        }.bufferedReader().readText()

    @Test
    fun `decodes a real load list`() {
        val envelope = json.decodeFromString(
            Envelope.serializer(ListSerializer(Load.serializer())),
            fixture("loads.json"),
        )

        val loads = assertNotNull(envelope.data).let { envelope.data!! }
        assertTrue("fixture should carry loads", loads.isNotEmpty())

        val load = loads.first()
        assertTrue(load.referenceNo.startsWith("LD-"))
        assertTrue("price is in minor units", load.priceAmount > 0)

        // The two the list hydration exists to provide; a regression on the
        // server side blanks the whole corridor.
        assertNotNull("pickup address", load.pickupAddress)
        assertNotNull("delivery address", load.deliveryAddress)
        assertNotNull("shipper", load.company)
        assertTrue(load.pickupAddress!!.shortLabel != "—")

        // The field that broke: a fractional rate.
        assertNotNull("price per km", load.pricePerKm)
        assertTrue(load.pricePerKm!! > 0)
    }

    @Test
    fun `decodes the real region vocabulary`() {
        val envelope = json.decodeFromString(
            Envelope.serializer(ListSerializer(Region.serializer())),
            fixture("regions.json"),
        )

        val regions = envelope.data.orEmpty()
        assertEquals("Uzbekistan has 14 regions", 14, regions.size)
        // Localised server-side; the client must never translate these.
        assertTrue(regions.all { it.name.isNotEmpty() && it.id.isNotEmpty() })
    }

    @Test
    fun `decodes the real vehicle type vocabulary`() {
        val envelope = json.decodeFromString(
            Envelope.serializer(ListSerializer(VehicleTypeOption.serializer())),
            fixture("vehicle-types.json"),
        )

        val types = envelope.data.orEmpty()
        assertTrue(types.isNotEmpty())
        assertTrue(types.any { it.code == "tent" })
        assertTrue(types.all { it.name.isNotEmpty() })
    }
}
