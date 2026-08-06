package uz.loadify.feature.driver.ui

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import uz.loadify.core.error.ApiException
import uz.loadify.core.ui.UiState
import uz.loadify.core.ui.loadState
import uz.loadify.feature.auth.data.DriverRepository
import uz.loadify.feature.driver.domain.DriverProfile
import uz.loadify.feature.driver.domain.Wallet
import javax.inject.Inject

data class ProfileUiState(
    val profile: UiState<DriverProfile> = UiState.Loading,
    val wallet: UiState<Wallet> = UiState.Loading,
    val togglingStatus: Boolean = false,
    val error: String? = null,
)

@HiltViewModel
class ProfileViewModel @Inject constructor(
    private val drivers: DriverRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(ProfileUiState())
    val state: StateFlow<ProfileUiState> = _state.asStateFlow()

    init {
        refresh()
    }

    fun refresh() {
        viewModelScope.launch {
            _state.update { it.copy(profile = loadState { drivers.profile() }) }
        }
        viewModelScope.launch {
            _state.update { it.copy(wallet = loadState { drivers.wallet() }) }
        }
    }

    fun setOnline(online: Boolean, genericError: String) {
        _state.update { it.copy(togglingStatus = true, error = null) }
        viewModelScope.launch {
            try {
                val updated = drivers.setStatus(online)
                _state.update { it.copy(profile = UiState.Success(updated)) }
            } catch (e: ApiException) {
                _state.update { it.copy(error = e.message) }
            } catch (_: Exception) {
                _state.update { it.copy(error = genericError) }
            } finally {
                _state.update { it.copy(togglingStatus = false) }
            }
        }
    }

    fun consumeError() = _state.update { it.copy(error = null) }
}
