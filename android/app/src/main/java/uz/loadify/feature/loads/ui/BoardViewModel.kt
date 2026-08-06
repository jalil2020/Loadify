package uz.loadify.feature.loads.ui

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import uz.loadify.core.error.ApiException
import uz.loadify.core.error.SessionExpiredException
import uz.loadify.core.network.Paged
import uz.loadify.core.ui.UiState
import uz.loadify.core.ui.loadState
import uz.loadify.feature.loads.data.LoadRepository
import uz.loadify.feature.loads.domain.Load
import uz.loadify.feature.loads.domain.LoadFilters
import uz.loadify.feature.loads.domain.Region
import uz.loadify.feature.loads.domain.VehicleTypeOption
import javax.inject.Inject

data class BoardUiState(
    val loads: UiState<Paged<Load>> = UiState.Loading,
    val filters: LoadFilters = LoadFilters(),
    val regions: List<Region> = emptyList(),
    val vehicleTypes: List<VehicleTypeOption> = emptyList(),
    val reservingId: String? = null,
    val refreshing: Boolean = false,
)

/** A one-shot message for the snackbar, consumed once shown. */
data class BoardMessage(val text: String, val isError: Boolean)

@HiltViewModel
class BoardViewModel @Inject constructor(
    private val loads: LoadRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(BoardUiState())
    val state: StateFlow<BoardUiState> = _state.asStateFlow()

    private val _messages = MutableStateFlow<BoardMessage?>(null)
    val messages: StateFlow<BoardMessage?> = _messages.asStateFlow()

    init {
        refresh()
        loadReferenceData()
    }

    fun refresh() {
        viewModelScope.launch {
            _state.update { it.copy(refreshing = it.loads is UiState.Success) }
            val result = loadState { loads.search(_state.value.filters) }
            _state.update { it.copy(loads = result, refreshing = false) }
        }
    }

    /**
     * Reference vocabularies are localised server-side, so they are re-fetched
     * when the language changes — translating them again on the client would
     * guarantee the two drift.
     */
    fun loadReferenceData() {
        viewModelScope.launch {
            val regions = runCatching { loads.regions() }.getOrDefault(emptyList())
            val types = runCatching { loads.vehicleTypes() }.getOrDefault(emptyList())
            _state.update { it.copy(regions = regions, vehicleTypes = types) }
        }
    }

    /** Any filter change resets to page one; the old page would often be empty. */
    fun updateFilters(transform: (LoadFilters) -> LoadFilters) {
        _state.update { it.copy(filters = transform(it.filters).copy(page = 1)) }
        refresh()
    }

    fun clearFilters() = updateFilters { LoadFilters() }

    fun reserve(load: Load, successMessage: String, genericError: String) {
        _state.update { it.copy(reservingId = load.id) }
        viewModelScope.launch {
            try {
                loads.reserve(load.id)
                _messages.value = BoardMessage(successMessage, isError = false)
                // The load leaves the published board on success, so the list
                // must re-fetch rather than patch the card in place.
                refresh()
            } catch (e: ApiException) {
                _messages.value = BoardMessage(e.message, isError = true)
            } catch (_: SessionExpiredException) {
                // The router reacts to the cleared session; nothing to say here.
            } catch (_: Exception) {
                _messages.value = BoardMessage(genericError, isError = true)
            } finally {
                _state.update { it.copy(reservingId = null) }
            }
        }
    }

    fun consumeMessage() {
        _messages.value = null
    }
}

@HiltViewModel
class LoadDetailViewModel @Inject constructor(
    private val loads: LoadRepository,
) : ViewModel() {

    private val _state = MutableStateFlow<UiState<Load>>(UiState.Loading)
    val state: StateFlow<UiState<Load>> = _state.asStateFlow()

    private val _reserving = MutableStateFlow(false)
    val reserving: StateFlow<Boolean> = _reserving.asStateFlow()

    private val _messages = MutableStateFlow<BoardMessage?>(null)
    val messages: StateFlow<BoardMessage?> = _messages.asStateFlow()

    private var loadId: String = ""

    fun load(id: String) {
        loadId = id
        viewModelScope.launch { _state.value = loadState { loads.byId(id) } }
    }

    fun reserve(successMessage: String, genericError: String) {
        if (loadId.isEmpty()) return
        _reserving.value = true
        viewModelScope.launch {
            try {
                loads.reserve(loadId)
                _messages.value = BoardMessage(successMessage, isError = false)
                _state.value = loadState { loads.byId(loadId) }
            } catch (e: ApiException) {
                _messages.value = BoardMessage(e.message, isError = true)
            } catch (_: SessionExpiredException) {
                // Handled by the session-driven redirect.
            } catch (_: Exception) {
                _messages.value = BoardMessage(genericError, isError = true)
            } finally {
                _reserving.value = false
            }
        }
    }

    fun consumeMessage() {
        _messages.value = null
    }
}
