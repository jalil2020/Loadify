package uz.loadify.core.error

/**
 * A failed request, carrying the backend's stable machine code.
 *
 * Callers branch on [code], never on [message] — messages are localised and
 * change, codes are part of the contract.
 */
class ApiException(
    val code: String,
    override val message: String,
    val status: Int,
    val fieldErrors: Map<String, String> = emptyMap(),
    val traceId: String? = null,
    cause: Throwable? = null,
) : Exception(message, cause) {

    val isValidation: Boolean get() = code == VALIDATION_ERROR
    val isAuth: Boolean get() = status == 401

    /**
     * The whole token family was revoked as a theft response. Retrying is
     * futile — the only correct move is to sign in again.
     */
    val isTokenReuse: Boolean get() = code == TOKEN_REUSE_DETECTED

    /** Worth offering a retry button for; a 4xx generally is not. */
    val isTransient: Boolean get() = status >= 500 || status == 0 || status == 429

    companion object {
        const val VALIDATION_ERROR = "VALIDATION_ERROR"
        const val TOKEN_REUSE_DETECTED = "TOKEN_REUSE_DETECTED"
        const val NETWORK_ERROR = "NETWORK_ERROR"
        const val MALFORMED_RESPONSE = "MALFORMED_RESPONSE"

        fun network(cause: Throwable): ApiException = ApiException(
            code = NETWORK_ERROR,
            message = "Could not reach Loadify. Check your connection.",
            status = 0,
            cause = cause,
        )

        fun malformed(path: String): ApiException = ApiException(
            code = MALFORMED_RESPONSE,
            message = "The server returned an unreadable response for $path",
            status = 0,
        )
    }
}

/**
 * Raised when the session is gone for good and the UI must return to login.
 *
 * Distinct from a 401: a 401 is recoverable by refreshing, this is what is left
 * when refreshing has already been tried and failed.
 */
class SessionExpiredException : Exception("Session expired")
