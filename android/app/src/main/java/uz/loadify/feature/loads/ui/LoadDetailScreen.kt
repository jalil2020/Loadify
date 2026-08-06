package uz.loadify.feature.loads.ui

import android.content.Intent
import android.net.Uri
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
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.ArrowBack
import androidx.compose.material.icons.rounded.Phone
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import uz.loadify.R
import uz.loadify.core.format.Fmt
import uz.loadify.core.ui.ErrorView
import uz.loadify.core.ui.SkeletonList
import uz.loadify.core.ui.StatusBadge
import uz.loadify.core.ui.UiState
import uz.loadify.core.ui.statusColor
import uz.loadify.feature.auth.domain.CurrentUser

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun LoadDetailScreen(
    loadId: String,
    user: CurrentUser?,
    fmt: Fmt,
    onBack: () -> Unit,
    viewModel: LoadDetailViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val reserving by viewModel.reserving.collectAsStateWithLifecycle()
    val message by viewModel.messages.collectAsStateWithLifecycle()
    val snackbar = remember { SnackbarHostState() }
    val context = LocalContext.current

    val reservedText = stringResource(R.string.board_reserved)
    val genericError = stringResource(R.string.common_error)
    val canReserve = user?.can("bookings.create") == true

    LaunchedEffect(loadId) { viewModel.load(loadId) }

    LaunchedEffect(message) {
        message?.let {
            snackbar.showSnackbar(it.text)
            viewModel.consumeMessage()
        }
    }

    val load = (state as? UiState.Success)?.value

    Scaffold(
        snackbarHost = { SnackbarHost(snackbar) },
        topBar = {
            TopAppBar(
                title = { Text(load?.referenceNo.orEmpty()) },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(
                            Icons.AutoMirrored.Rounded.ArrowBack,
                            contentDescription = stringResource(R.string.common_back),
                        )
                    }
                },
            )
        },
        bottomBar = {
            if (load != null) {
                Surface(shadowElevation = 8.dp) {
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(16.dp),
                    ) {
                        Column(Modifier.weight(1f)) {
                            Text(
                                fmt.money(load.priceAmount, load.priceCurrency),
                                style = MaterialTheme.typography.titleLarge,
                                fontWeight = FontWeight.Bold,
                                color = MaterialTheme.colorScheme.secondary,
                            )
                            load.pricePerKm?.let {
                                Text(
                                    "${fmt.money(it, load.priceCurrency)} " +
                                        stringResource(R.string.board_per_km),
                                    style = MaterialTheme.typography.labelSmall,
                                )
                            }
                        }
                        Spacer(Modifier.width(12.dp))
                        Button(
                            onClick = { viewModel.reserve("${load.referenceNo}: $reservedText", genericError) },
                            enabled = canReserve && !load.isBooked && !reserving,
                            modifier = Modifier.height(48.dp),
                        ) {
                            if (reserving) {
                                CircularProgressIndicator(
                                    strokeWidth = 2.dp,
                                    modifier = Modifier.size(18.dp),
                                    color = MaterialTheme.colorScheme.onPrimary,
                                )
                            } else {
                                Text(
                                    stringResource(
                                        if (load.isBooked) R.string.board_reserved else R.string.board_reserve,
                                    ),
                                )
                            }
                        }
                    }
                }
            }
        },
    ) { padding ->
        when (val current = state) {
            is UiState.Loading -> SkeletonList(count = 3, itemHeight = 120)

            is UiState.Failure -> ErrorView(
                current.error,
                onRetry = { viewModel.load(loadId) },
                modifier = Modifier.padding(padding),
            )

            is UiState.Success -> {
                val l = current.value
                Column(
                    Modifier
                        .fillMaxSize()
                        .padding(padding)
                        .verticalScroll(rememberScrollState())
                        .padding(16.dp),
                ) {
                    Row(verticalAlignment = Alignment.Top) {
                        Text(
                            l.title,
                            style = MaterialTheme.typography.headlineSmall,
                            fontWeight = FontWeight.Bold,
                            modifier = Modifier.weight(1f),
                        )
                        StatusBadge(l.status, statusColor(l.status, MaterialTheme.colorScheme))
                    }
                    if (!l.description.isNullOrEmpty()) {
                        Spacer(Modifier.height(6.dp))
                        Text(l.description, style = MaterialTheme.typography.bodyMedium)
                    }

                    Spacer(Modifier.height(16.dp))
                    Section(stringResource(R.string.load_route)) {
                        DetailRow(
                            stringResource(R.string.filter_from),
                            l.pickupAddress?.formatted ?: Fmt.EM_DASH,
                        )
                        DetailRow(
                            stringResource(R.string.filter_to),
                            l.deliveryAddress?.formatted ?: Fmt.EM_DASH,
                        )
                        DetailRow(
                            stringResource(R.string.load_pickup_date),
                            fmt.date(l.pickupFrom),
                        )
                        DetailRow(
                            stringResource(R.string.load_distance),
                            fmt.distance(l.distanceKm),
                        )
                    }

                    Spacer(Modifier.height(12.dp))
                    Section(stringResource(R.string.load_cargo)) {
                        DetailRow(stringResource(R.string.load_weight), fmt.weight(l.weightKg))
                        l.requiredVehicleType?.let {
                            DetailRow(stringResource(R.string.filter_vehicle_type), it)
                        }
                        if (l.requiresAdr) {
                            DetailRow(
                                stringResource(R.string.load_requirements),
                                stringResource(R.string.load_adr),
                            )
                        }
                        if (l.requiresRefrigeration) {
                            DetailRow(
                                stringResource(R.string.load_requirements),
                                stringResource(R.string.load_refrigerated),
                            )
                        }
                    }

                    Spacer(Modifier.height(12.dp))
                    Section(stringResource(R.string.load_shipper)) {
                        DetailRow("", l.company?.brandName ?: Fmt.EM_DASH)
                        DetailRow(
                            stringResource(R.string.profile_rating),
                            "★ %.1f".format(l.company?.rating ?: 0.0),
                        )
                        // Masked until a booking exists. Shown as an explicit
                        // state so the driver knows a number exists to be had.
                        DetailRow(
                            stringResource(R.string.common_call),
                            l.company?.phone?.let { Fmt.phone(it) }
                                ?: stringResource(R.string.board_contact_hidden),
                        )
                    }

                    l.company?.phone?.let { phone ->
                        Spacer(Modifier.height(12.dp))
                        OutlinedButton(
                            onClick = {
                                context.startActivity(
                                    Intent(Intent.ACTION_DIAL, Uri.parse("tel:$phone")),
                                )
                            },
                            modifier = Modifier.fillMaxWidth(),
                        ) {
                            Icon(Icons.Rounded.Phone, contentDescription = null)
                            Spacer(Modifier.width(8.dp))
                            Text(stringResource(R.string.common_call))
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun Section(title: String, content: @Composable () -> Unit) {
    Card(Modifier.fillMaxWidth()) {
        Column(Modifier.padding(14.dp)) {
            Text(title, style = MaterialTheme.typography.titleSmall)
            Spacer(Modifier.height(8.dp))
            content()
        }
    }
}

@Composable
private fun DetailRow(label: String, value: String) {
    Row(Modifier.padding(vertical = 4.dp)) {
        Text(
            label,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.width(96.dp),
        )
        Text(value, style = MaterialTheme.typography.bodyMedium, modifier = Modifier.weight(1f))
    }
}
