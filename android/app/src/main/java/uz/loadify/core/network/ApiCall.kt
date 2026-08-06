package uz.loadify.core.network

import kotlinx.serialization.json.Json
import retrofit2.Response
import uz.loadify.core.error.ApiException
import uz.loadify.core.error.SessionExpiredException
import java.io.IOException

/**
 * Turns a Retrofit [Response] into either the payload or a typed failure.
 *
 * Every repository goes through here so error handling exists in one place: a
 * screen sees an [ApiException] carrying the backend's machine code, or a
 * [SessionExpiredException] telling it to send the driver back to login.
 */
class ApiCall(private val json: Json) {

    suspend fun <T : Any> one(path: String, block: suspend () -> Response<Envelope<T>>): T {
        val envelope = execute(path, block)
        return envelope.data ?: throw ApiException.malformed(path)
    }

    suspend fun <T : Any> page(
        path: String,
        block: suspend () -> Response<Envelope<List<T>>>,
    ): Paged<T> {
        val envelope = execute(path, block)
        return Paged(
            items = envelope.data.orEmpty(),
            meta = envelope.meta ?: PageMeta(total = envelope.data?.size ?: 0),
        )
    }

    suspend fun <T : Any> list(
        path: String,
        block: suspend () -> Response<Envelope<List<T>>>,
    ): List<T> = execute(path, block).data.orEmpty()

    /** For endpoints whose result is the status code, not a body. */
    suspend fun unit(path: String, block: suspend () -> Response<Envelope<Unit>>) {
        execute(path, block)
    }

    private suspend fun <T> execute(
        path: String,
        block: suspend () -> Response<Envelope<T>>,
    ): Envelope<T> {
        val response = try {
            block()
        } catch (e: IOException) {
            throw ApiException.network(e)
        }

        if (response.isSuccessful) {
            // A 204 has no body; callers that need one hit the null check above.
            return response.body() ?: Envelope()
        }

        val error = parseError(response)

        // A 401 here means the authenticator already tried to refresh and could
        // not — the session is genuinely gone, not merely stale.
        if (response.code() == 401 || error.isTokenReuse) {
            throw SessionExpiredException()
        }
        throw error
    }

    private fun parseError(response: Response<*>): ApiException {
        val raw = try {
            response.errorBody()?.string()
        } catch (_: IOException) {
            null
        }
        val parsed = raw?.let {
            try {
                json.decodeFromString(ErrorEnvelope.serializer(), it)
            } catch (_: IllegalArgumentException) {
                // An error body we cannot parse is still an error; losing the
                // status code because the shape surprised us would be worse.
                null
            }
        }
        val body = parsed?.error
        return ApiException(
            code = body?.code ?: "UNKNOWN",
            message = body?.message ?: "Something went wrong",
            status = response.code(),
            fieldErrors = body?.details.orEmpty().associate { it.field to it.message },
            traceId = parsed?.traceId,
        )
    }
}
