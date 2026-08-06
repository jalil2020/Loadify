package uz.loadify.feature.auth.ui

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch
import uz.loadify.core.error.ApiException
import uz.loadify.feature.auth.data.AuthRepository
import uz.loadify.feature.auth.domain.CurrentUser
import javax.inject.Inject

/**
 * The session, shared by every screen.
 *
 * Driven by the store's own flow rather than by whoever last called signIn, so
 * a refresh that adopts updated permissions — or a forced sign-out from a 401 —
 * propagates everywhere without anyone re-fetching.
 */
@HiltViewModel
class SessionViewModel @Inject constructor(
    private val auth: AuthRepository,
) : ViewModel() {

    val user: StateFlow<CurrentUser?> = auth.session
        .map { it?.user }
        .stateIn(viewModelScope, SharingStarted.Eagerly, null)

    val locale: StateFlow<String> = auth.locale

    fun signOut() {
        viewModelScope.launch { auth.signOut() }
    }

    fun setLocale(tag: String) = auth.setLocale(tag)
}

data class LoginUiState(
    val phone: String = "+998",
    val password: String = "",
    val phoneError: String? = null,
    val passwordError: String? = null,
    val formError: String? = null,
    val submitting: Boolean = false,
    val passwordVisible: Boolean = false,
)

@HiltViewModel
class LoginViewModel @Inject constructor(
    private val auth: AuthRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(LoginUiState())
    val state: StateFlow<LoginUiState> = _state.asStateFlow()

    fun onPhoneChange(value: String) {
        _state.value = _state.value.copy(phone = value, phoneError = null, formError = null)
    }

    fun onPasswordChange(value: String) {
        _state.value = _state.value.copy(password = value, passwordError = null, formError = null)
    }

    fun togglePasswordVisible() {
        _state.value = _state.value.copy(passwordVisible = !_state.value.passwordVisible)
    }

    /**
     * Validates locally, then signs in.
     *
     * [invalidPhone] and [shortPassword] are resource ids resolved by the
     * screen: the ViewModel decides *that* a field is wrong, the UI layer
     * decides how to say it, so nothing here holds a Context.
     */
    fun submit(invalidPhone: String, shortPassword: String, onSuccess: () -> Unit) {
        val current = _state.value
        val phoneBad = !PHONE.matches(current.phone.trim())
        val passwordBad = current.password.length < MIN_PASSWORD

        if (phoneBad || passwordBad) {
            _state.value = current.copy(
                phoneError = if (phoneBad) invalidPhone else null,
                passwordError = if (passwordBad) shortPassword else null,
            )
            return
        }

        _state.value = current.copy(submitting = true, formError = null)
        viewModelScope.launch {
            try {
                auth.signIn(current.phone.trim(), current.password)
                onSuccess()
            } catch (e: ApiException) {
                // Field errors belong on the field; only what cannot be
                // attributed to an input becomes a form-level message.
                _state.value = _state.value.copy(
                    submitting = false,
                    phoneError = e.fieldErrors["phone"],
                    passwordError = e.fieldErrors["password"],
                    formError = if (e.fieldErrors.isEmpty()) e.message else null,
                )
            } catch (e: Exception) {
                _state.value = _state.value.copy(submitting = false, formError = e.message)
            }
        }
    }

    private companion object {
        val PHONE = Regex("""^\+998\d{9}$""")
        const val MIN_PASSWORD = 8
    }
}
