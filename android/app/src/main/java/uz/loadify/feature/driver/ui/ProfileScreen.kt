package uz.loadify.feature.driver.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.Logout
import androidx.compose.material.icons.outlined.LocalShipping
import androidx.compose.material.icons.rounded.Language
import androidx.compose.material3.Card
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.ListItem
import androidx.compose.material3.ListItemDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import uz.loadify.R
import uz.loadify.core.format.Fmt
import uz.loadify.core.ui.ErrorView
import uz.loadify.core.ui.SUPPORTED_LOCALES
import uz.loadify.core.ui.StatusBadge
import uz.loadify.core.ui.UiState
import uz.loadify.core.ui.localeLabel
import uz.loadify.feature.auth.domain.CurrentUser
import uz.loadify.feature.driver.domain.DriverStatus

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ProfileScreen(
    user: CurrentUser?,
    fmt: Fmt,
    currentLocale: String,
    onLocaleChange: (String) -> Unit,
    onSignOut: () -> Unit,
    viewModel: ProfileViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val snackbar = remember { SnackbarHostState() }
    val genericError = stringResource(R.string.common_error)
    val scheme = MaterialTheme.colorScheme

    LaunchedEffect(state.error) {
        state.error?.let {
            snackbar.showSnackbar(it)
            viewModel.consumeError()
        }
    }

    Scaffold(
        snackbarHost = { SnackbarHost(snackbar) },
        topBar = { TopAppBar(title = { Text(stringResource(R.string.nav_profile)) }) },
    ) { padding ->
        Column(
            Modifier
                .fillMaxSize()
                .padding(padding)
                .verticalScroll(rememberScrollState())
                .padding(16.dp),
        ) {
            Header(user, state, fmt)

            // Only a driver has an availability status to set. A company user
            // signed in on the phone sees the rest without a toggle that would
            // mean nothing for them.
            if (user?.isDriver == true) {
                Spacer(Modifier.height(12.dp))
                Card(Modifier.fillMaxWidth()) {
                    when (val profile = state.profile) {
                        is UiState.Success -> {
                            val status = profile.value.driverStatus
                            ListItem(
                                headlineContent = {
                                    Text(stringResource(R.string.profile_accepting_work))
                                },
                                supportingContent = { Text(stringResource(status.labelRes)) },
                                trailingContent = {
                                    Switch(
                                        checked = status.isAvailable,
                                        // `busy` is server-assigned while a trip
                                        // runs; letting the driver flip it here
                                        // would fight the trip state machine.
                                        enabled = !state.togglingStatus &&
                                            status != DriverStatus.BUSY,
                                        onCheckedChange = {
                                            viewModel.setOnline(it, genericError)
                                        },
                                    )
                                },
                                colors = ListItemDefaults.colors(
                                    containerColor = scheme.surface,
                                ),
                            )
                        }
                        is UiState.Failure -> ErrorView(
                            profile.error,
                            onRetry = viewModel::refresh,
                            modifier = Modifier.height(120.dp),
                        )
                        is UiState.Loading -> ListItem(
                            headlineContent = { Text(stringResource(R.string.common_loading)) },
                        )
                    }
                }
            }

            Spacer(Modifier.height(12.dp))
            Card(Modifier.fillMaxWidth()) {
                Column(Modifier.padding(14.dp)) {
                    Text(
                        stringResource(R.string.profile_wallet),
                        style = MaterialTheme.typography.titleSmall,
                    )
                    Spacer(Modifier.height(10.dp))
                    when (val wallet = state.wallet) {
                        is UiState.Success -> Row {
                            Metric(
                                stringResource(R.string.profile_available),
                                fmt.money(wallet.value.available, wallet.value.currency),
                                emphasis = true,
                                modifier = Modifier.weight(1f),
                            )
                            Metric(
                                stringResource(R.string.profile_held),
                                fmt.money(wallet.value.heldBalance, wallet.value.currency),
                                modifier = Modifier.weight(1f),
                            )
                        }
                        is UiState.Failure -> TextButton(onClick = viewModel::refresh) {
                            Text(stringResource(R.string.common_retry))
                        }
                        is UiState.Loading -> Text(stringResource(R.string.common_loading))
                    }
                }
            }

            val vehicles = state.profile.valueOrNull?.vehicles.orEmpty()
            if (vehicles.isNotEmpty()) {
                Spacer(Modifier.height(12.dp))
                Card(Modifier.fillMaxWidth()) {
                    Column(Modifier.padding(14.dp)) {
                        Text(
                            stringResource(R.string.profile_vehicles),
                            style = MaterialTheme.typography.titleSmall,
                        )
                        Spacer(Modifier.height(8.dp))
                        vehicles.forEach { vehicle ->
                            Row(
                                verticalAlignment = Alignment.CenterVertically,
                                modifier = Modifier.padding(vertical = 5.dp),
                            ) {
                                Icon(
                                    Icons.Outlined.LocalShipping,
                                    contentDescription = null,
                                    tint = scheme.onSurfaceVariant,
                                    modifier = Modifier.size(18.dp),
                                )
                                Spacer(Modifier.width(10.dp))
                                Text(
                                    "${vehicle.label} · ${vehicle.plateNumber}",
                                    style = MaterialTheme.typography.bodyMedium,
                                    maxLines = 1,
                                    overflow = TextOverflow.Ellipsis,
                                    modifier = Modifier.weight(1f),
                                )
                                Text(
                                    fmt.weight(vehicle.capacityKg),
                                    style = MaterialTheme.typography.labelSmall,
                                    color = scheme.onSurfaceVariant,
                                )
                            }
                        }
                    }
                }
            }

            Spacer(Modifier.height(12.dp))
            Card(Modifier.fillMaxWidth()) {
                Column {
                    ListItem(
                        leadingContent = {
                            Icon(Icons.Rounded.Language, contentDescription = null)
                        },
                        headlineContent = { Text(stringResource(R.string.profile_language)) },
                        supportingContent = {
                            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                SUPPORTED_LOCALES.forEach { tag ->
                                    FilterChip(
                                        selected = currentLocale == tag,
                                        onClick = { onLocaleChange(tag) },
                                        label = { Text(localeLabel(tag)) },
                                    )
                                }
                            }
                        },
                        colors = ListItemDefaults.colors(containerColor = scheme.surface),
                    )
                    HorizontalDivider()
                    ListItem(
                        leadingContent = {
                            Icon(
                                Icons.AutoMirrored.Rounded.Logout,
                                contentDescription = null,
                                tint = scheme.error,
                            )
                        },
                        headlineContent = {
                            Text(
                                stringResource(R.string.profile_sign_out),
                                color = scheme.error,
                            )
                        },
                        modifier = Modifier.clickable(onClick = onSignOut),
                        colors = ListItemDefaults.colors(containerColor = scheme.surface),
                    )
                }
            }
        }
    }
}

@Composable
private fun Header(user: CurrentUser?, state: ProfileUiState, fmt: Fmt) {
    val scheme = MaterialTheme.colorScheme
    val profile = state.profile.valueOrNull

    Card(Modifier.fillMaxWidth()) {
        Column(Modifier.padding(16.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Box(
                    Modifier
                        .size(52.dp)
                        .background(scheme.primary, CircleShape),
                    contentAlignment = Alignment.Center,
                ) {
                    Text(
                        user?.initials ?: "?",
                        color = scheme.onPrimary,
                        fontWeight = FontWeight.Bold,
                        style = MaterialTheme.typography.titleMedium,
                    )
                }
                Spacer(Modifier.width(14.dp))
                Column(Modifier.weight(1f)) {
                    Text(
                        user?.fullName ?: Fmt.EM_DASH,
                        style = MaterialTheme.typography.titleMedium,
                        fontWeight = FontWeight.SemiBold,
                    )
                    Text(
                        Fmt.phone(user?.phone),
                        style = MaterialTheme.typography.bodySmall,
                        color = scheme.onSurfaceVariant,
                    )
                }
                if (profile != null) {
                    StatusBadge(
                        stringResource(
                            if (profile.isVerified) {
                                R.string.profile_verified
                            } else {
                                R.string.profile_unverified
                            },
                        ),
                        if (profile.isVerified) scheme.secondary else scheme.outline,
                    )
                }
            }

            if (profile != null) {
                Spacer(Modifier.height(16.dp))
                Row {
                    Metric(
                        stringResource(R.string.profile_rating),
                        "★ %.1f".format(profile.rating),
                        modifier = Modifier.weight(1f),
                    )
                    Metric(
                        stringResource(R.string.profile_completed_trips),
                        fmt.number(profile.completedTrips),
                        modifier = Modifier.weight(1f),
                    )
                }
            }
        }
    }
}

@Composable
private fun Metric(
    label: String,
    value: String,
    modifier: Modifier = Modifier,
    emphasis: Boolean = false,
) {
    Column(modifier) {
        Text(
            label,
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Spacer(Modifier.height(2.dp))
        Text(
            value,
            style = MaterialTheme.typography.titleMedium,
            fontWeight = FontWeight.Bold,
            color = if (emphasis) {
                MaterialTheme.colorScheme.secondary
            } else {
                MaterialTheme.colorScheme.onSurface
            },
        )
    }
}
