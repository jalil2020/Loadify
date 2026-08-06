package uz.loadify.feature.trips.ui

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
import uz.loadify.feature.bookings.domain.Booking
import uz.loadify.feature.trips.data.BookingRepository
import uz.loadify.feature.trips.data.TripRepository
import uz.loadify.feature.trips.domain.Trip
import uz.loadify.feature.trips.domain.TripStage
import javax.inject.Inject

data class TripsUiState(
    val trips: UiState<Paged<Trip>> = UiState.Loading,
    val status: String = "active",
    val advancingId: String? = null,
    val error: String? = null,
)

@HiltViewModel
class TripsViewModel @Inject constructor(
    private val trips: TripRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(TripsUiState())
    val state: StateFlow<TripsUiState> = _state.asStateFlow()

    init {
        refresh()
    }

    fun refresh() {
        viewModelScope.launch {
            val result = loadState { trips.list(status = _state.value.status) }
            _state.update { it.copy(trips = result) }
        }
    }

    fun setStatus(status: String) {
        _state.update { it.copy(status = status, trips = UiState.Loading) }
        refresh()
    }

    fun advance(trip: Trip, to: TripStage, genericError: String) {
        _state.update { it.copy(advancingId = trip.id, error = null) }
        viewModelScope.launch {
            try {
                trips.advance(trip.id, to)
                refresh()
            } catch (e: ApiException) {
                _state.update { it.copy(error = e.message) }
            } catch (_: SessionExpiredException) {
                // Handled by the session-driven redirect.
            } catch (_: Exception) {
                _state.update { it.copy(error = genericError) }
            } finally {
                _state.update { it.copy(advancingId = null) }
            }
        }
    }

    fun consumeError() = _state.update { it.copy(error = null) }
}

data class BookingsUiState(
    val bookings: UiState<Paged<Booking>> = UiState.Loading,
    val busyId: String? = null,
    val error: String? = null,
)

@HiltViewModel
class BookingsViewModel @Inject constructor(
    private val bookings: BookingRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(BookingsUiState())
    val state: StateFlow<BookingsUiState> = _state.asStateFlow()

    init {
        refresh()
    }

    fun refresh() {
        viewModelScope.launch {
            val result = loadState { bookings.list() }
            _state.update { it.copy(bookings = result) }
        }
    }

    fun respond(booking: Booking, accept: Boolean, genericError: String) =
        run(booking.id, genericError) { bookings.respond(booking.id, accept) }

    fun withdraw(booking: Booking, genericError: String) =
        run(booking.id, genericError) { bookings.withdraw(booking.id) }

    private fun run(id: String, genericError: String, action: suspend () -> Unit) {
        _state.update { it.copy(busyId = id, error = null) }
        viewModelScope.launch {
            try {
                action()
                refresh()
            } catch (e: ApiException) {
                _state.update { it.copy(error = e.message) }
            } catch (_: SessionExpiredException) {
                // Handled by the session-driven redirect.
            } catch (_: Exception) {
                _state.update { it.copy(error = genericError) }
            } finally {
                _state.update { it.copy(busyId = null) }
            }
        }
    }

    fun consumeError() = _state.update { it.copy(error = null) }
}
