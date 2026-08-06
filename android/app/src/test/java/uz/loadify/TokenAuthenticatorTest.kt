package uz.loadify

import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.mockwebserver.Dispatcher
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import uz.loadify.core.network.Anonymous
import uz.loadify.core.network.AuthInterceptor
import uz.loadify.core.network.TokenAuthenticator
import uz.loadify.core.network.TokenRefresher
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger

/**
 * Exercises the real OkHttp authenticator against a real server socket, because
 * the behaviour under test is a concurrency property — a hand-rolled fake would
 * only prove the fake serialises.
 */
class TokenAuthenticatorTest {

    private lateinit var server: MockWebServer

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun client(
        sessions: FakeSessionHolder,
        refresher: TokenRefresher,
    ): OkHttpClient = OkHttpClient.Builder()
        .addInterceptor(AuthInterceptor(sessions))
        .authenticator(TokenAuthenticator(sessions, refresher))
        .build()

    private fun get(client: OkHttpClient, path: String) =
        client.newCall(Request.Builder().url(server.url(path)).build()).execute()

    /** 401 for the stale token, 200 for anything else. */
    private fun rejectStaleDispatcher(staleToken: String) = object : Dispatcher() {
        override fun dispatch(request: RecordedRequest): MockResponse =
            if (request.getHeader("Authorization") == "Bearer $staleToken") {
                MockResponse().setResponseCode(401)
                    .setBody("""{"error":{"code":"TOKEN_EXPIRED","message":"expired"}}""")
            } else {
                MockResponse().setResponseCode(200).setBody("""{"success":true,"data":{}}""")
            }
    }

    @Test
    fun `a 401 refreshes once and replays the request with the new token`() {
        val sessions = FakeSessionHolder(FakeSessionHolder.session("stale"))
        val refreshes = AtomicInteger(0)
        server.dispatcher = rejectStaleDispatcher("stale")

        val client = client(sessions) {
            refreshes.incrementAndGet()
            FakeSessionHolder.session("fresh", "refresh-2")
        }

        val response = get(client, "/trips")

        assertEquals(200, response.code)
        assertEquals(1, refreshes.get())
        assertEquals("fresh", sessions.current?.accessToken)

        // The replay must carry the NEW token; sending the stale one again is
        // the shape of the bug this exists to catch.
        val requests = generateSequence { server.takeRequest(1, TimeUnit.SECONDS) }.toList()
        assertEquals("Bearer fresh", requests.last().getHeader("Authorization"))
    }

    @Test
    fun `concurrent 401s share a single refresh`() {
        val sessions = FakeSessionHolder(FakeSessionHolder.session("stale"))
        val refreshes = AtomicInteger(0)
        server.dispatcher = rejectStaleDispatcher("stale")

        val client = client(sessions) {
            refreshes.incrementAndGet()
            // A real refresh takes a round trip. Without the lock the other
            // callers slip in during this gap and each fire their own, which is
            // what trips the backend's reuse detection.
            Thread.sleep(120)
            FakeSessionHolder.session("fresh", "refresh-2")
        }

        val threads = 4
        val ready = CountDownLatch(threads)
        val go = CountDownLatch(1)
        val codes = java.util.Collections.synchronizedList(mutableListOf<Int>())

        val workers = (1..threads).map { i ->
            Thread {
                ready.countDown()
                go.await()
                codes += get(client, "/r$i").code
            }.apply { start() }
        }

        ready.await(5, TimeUnit.SECONDS)
        go.countDown()
        workers.forEach { it.join(15_000) }

        assertEquals(
            "four concurrent 401s must share one refresh",
            1,
            refreshes.get(),
        )
        assertEquals(listOf(200, 200, 200, 200), codes.sorted())
    }

    @Test
    fun `a refresher that cannot refresh clears the session and gives up`() {
        val sessions = FakeSessionHolder(FakeSessionHolder.session("stale"))
        server.dispatcher = rejectStaleDispatcher("stale")

        val client = client(sessions) { null }
        val response = get(client, "/trips")

        // OkHttp surfaces the original 401 once the authenticator declines.
        assertEquals(401, response.code)
        assertTrue(sessions.cleared)
        assertNull(sessions.current)
    }

    @Test
    fun `an anonymous request is never refreshed`() {
        val sessions = FakeSessionHolder(FakeSessionHolder.session("stale"))
        val refreshes = AtomicInteger(0)
        server.enqueue(
            MockResponse().setResponseCode(401)
                .setBody("""{"error":{"code":"UNAUTHORIZED","message":"nope"}}"""),
        )

        val client = client(sessions) {
            refreshes.incrementAndGet()
            FakeSessionHolder.session("fresh")
        }

        val response = client.newCall(
            Request.Builder()
                .url(server.url("/loads"))
                .tag(Anonymous::class.java, Anonymous)
                .build(),
        ).execute()

        assertEquals(401, response.code)
        // Refreshing on behalf of a request that deliberately carried no
        // credential would rotate a token nobody asked to use.
        assertEquals(0, refreshes.get())
    }

    @Test
    fun `an anonymous request carries the locale but no credential`() {
        val sessions = FakeSessionHolder(FakeSessionHolder.session("token"))
        server.enqueue(MockResponse().setResponseCode(200).setBody("{}"))

        val client = client(sessions) { null }
        client.newCall(
            Request.Builder()
                .url(server.url("/loads"))
                .tag(Anonymous::class.java, Anonymous)
                .build(),
        ).execute()

        val sent = server.takeRequest(2, TimeUnit.SECONDS)!!
        // Even holding a valid session, an anonymous call must not present it —
        // that is what lets a signed-in driver see the guest board.
        assertNull(sent.getHeader("Authorization"))
        // The marker is a tag, so it never reaches the wire at all.
        assertNull(sent.getHeader("X-Loadify-Anonymous"))
        assertEquals("uz", sent.getHeader("Accept-Language"))
    }

    @Test
    fun `an authenticated request carries the token`() {
        val sessions = FakeSessionHolder(FakeSessionHolder.session("token"))
        server.enqueue(MockResponse().setResponseCode(200).setBody("{}"))

        get(client(sessions) { null }, "/trips")

        val sent = server.takeRequest(2, TimeUnit.SECONDS)!!
        assertEquals("Bearer token", sent.getHeader("Authorization"))
    }
}
