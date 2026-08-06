package uz.loadify.core.network

import okhttp3.Authenticator
import okhttp3.Interceptor
import okhttp3.Request
import okhttp3.Response
import okhttp3.Route
import uz.loadify.core.storage.Session
import uz.loadify.core.storage.SessionHolder

/**
 * Marks a request that must go out unauthenticated (login, refresh, the board).
 *
 * A tag rather than a header, for two reasons. It never travels to the server,
 * and — the one that matters — a header the interceptor strips is invisible to
 * the authenticator, which sees the *modified* request. That version would have
 * refreshed on behalf of a deliberately anonymous call and replayed it with
 * credentials attached.
 */
object Anonymous

internal fun Request.isAnonymous(): Boolean = tag(Anonymous::class.java) != null

/**
 * Attaches the access token and the locale.
 *
 * The locale is sent on every request because reference vocabularies — region
 * names, vehicle types — are localised server-side; the client never translates
 * them itself, so the two cannot drift.
 */
class AuthInterceptor(private val sessions: SessionHolder) : Interceptor {
    override fun intercept(chain: Interceptor.Chain): Response {
        val request = chain.request()
        val builder = request.newBuilder()
            .header("Accept-Language", sessions.currentLocale)

        if (!request.isAnonymous()) {
            sessions.current?.accessToken?.let { builder.header("Authorization", "Bearer $it") }
        }
        return chain.proceed(builder.build())
    }
}

/**
 * Refreshes the token pair when a request comes back 401, then lets OkHttp
 * replay the original request.
 *
 * OkHttp calls this from whichever thread got the 401, so several can arrive at
 * once. Without the lock plus the already-rotated check below, a screen firing
 * four queries would send four refreshes; the backend rotates the refresh token
 * on first use, so the other three would present an already-rotated token and
 * trip its theft detection — signing the driver out for doing nothing wrong.
 */
class TokenAuthenticator(
    private val sessions: SessionHolder,
    private val refresher: TokenRefresher,
) : Authenticator {

    private val lock = Any()

    override fun authenticate(route: Route?, response: Response): Request? {
        if (response.request.isAnonymous()) return null

        // Two failures in a row on the same call means refreshing did not help.
        // Giving up here is what stops an authentication loop.
        if (responseCount(response) >= 2) return null

        val stale = response.request.header("Authorization")?.removePrefix("Bearer ")

        synchronized(lock) {
            val current = sessions.current ?: return null

            // Another thread refreshed while this one waited on the lock. Its
            // token is good, so replay with that rather than refreshing again.
            if (stale != null && current.accessToken != stale) {
                return response.request.retryWith(current.accessToken)
            }

            val refreshed: Session = refresher.refresh(current.refreshToken) ?: run {
                sessions.clear()
                return null
            }
            sessions.save(refreshed)
            return response.request.retryWith(refreshed.accessToken)
        }
    }

    private fun Request.retryWith(token: String): Request =
        newBuilder().header("Authorization", "Bearer $token").build()

    private fun responseCount(response: Response): Int {
        var count = 1
        var prior = response.priorResponse
        while (prior != null) {
            count++
            prior = prior.priorResponse
        }
        return count
    }
}

/**
 * Performs the refresh call.
 *
 * Separate from the authenticator so the blocking network call can be swapped
 * in tests, and so the authenticator holds no Retrofit knowledge.
 */
fun interface TokenRefresher {
    /** Returns the new session, or null if the refresh token is no longer good. */
    fun refresh(refreshToken: String): Session?
}
