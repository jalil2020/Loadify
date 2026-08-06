package uz.loadify.core.network

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/** The envelope every endpoint returns. */
@Serializable
data class Envelope<T>(
    val success: Boolean = true,
    val data: T? = null,
    val meta: PageMeta? = null,
)

@Serializable
data class PageMeta(
    val page: Int = 1,
    @SerialName("per_page") val perPage: Int = 0,
    val total: Int = 0,
    @SerialName("total_pages") val totalPages: Int = 1,
) {
    val hasMore: Boolean get() = page < totalPages
}

/** A page of results, with the metadata the board needs to show a total. */
data class Paged<T>(
    val items: List<T>,
    val meta: PageMeta,
)

@Serializable
data class ErrorEnvelope(
    val error: ErrorBody? = null,
    @SerialName("trace_id") val traceId: String? = null,
)

@Serializable
data class ErrorBody(
    val code: String = "UNKNOWN",
    val message: String = "Something went wrong",
    val details: List<FieldError> = emptyList(),
)

@Serializable
data class FieldError(
    val field: String = "",
    val message: String = "",
)
