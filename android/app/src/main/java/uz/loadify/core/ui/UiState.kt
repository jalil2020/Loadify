package uz.loadify.core.ui

/**
 * What every data-backed screen can be showing.
 *
 * Modelled as a sealed hierarchy rather than a struct of nullable fields so
 * "loading with a stale error still on screen" is not representable.
 */
sealed interface UiState<out T> {
    data object Loading : UiState<Nothing>
    data class Success<T>(val value: T) : UiState<T>
    data class Failure(val error: Throwable) : UiState<Nothing>

    val valueOrNull: T? get() = (this as? Success)?.value
}

/** Runs [block], mapping success and failure into a [UiState]. */
suspend inline fun <T> loadState(block: suspend () -> T): UiState<T> = try {
    UiState.Success(block())
} catch (e: Exception) {
    // Deliberately broad: a screen must render *something* for any failure,
    // and the specific type is preserved on the Failure for callers that care.
    UiState.Failure(e)
}
