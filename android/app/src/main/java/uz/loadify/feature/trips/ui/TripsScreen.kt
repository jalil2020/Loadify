package uz.loadify.feature.trips.ui

import android.Manifest
import android.content.Intent
import android.net.Uri
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.ArrowForward
import androidx.compose.material.icons.rounded.HourglassTop
import androidx.compose.material.icons.rounded.LocationDisabled
import androidx.compose.material.icons.rounded.MyLocation
import androidx.compose.material.icons.rounded.Phone
import androidx.compose.material.icons.rounded.Route
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilledTonalIconButton
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import uz.loadify.R
import uz.loadify.core.format.Fmt
import uz.loadify.core.ui.EmptyView
import uz.loadify.core.ui.ErrorView
import uz.loadify.core.ui.SkeletonList
import uz.loadify.core.ui.StatusBadge
import uz.loadify.core.ui.UiState
import uz.loadify.core.ui.statusColor
import uz.loadify.feature.trips.data.TripRepository
import uz.loadify.feature.trips.domain.Trip
import uz.loadify.feature.trips.domain.TripStage

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TripsScreen(
    fmt: Fmt,
    tripRepository: TripRepository,
    viewModel: TripsViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val snackbar = remember { SnackbarHostState() }
    val genericError = stringResource(R.string.common_error)

    LaunchedEffect(state.error) {
        state.error?.let {
            snackbar.showSnackbar(it)
            viewModel.consumeError()
        }
    }

    Scaffold(
        snackbarHost = { SnackbarHost(snackbar) },
        topBar = {
            Column {
                TopAppBar(title = { Text(stringResource(R.string.trips_title)) })
                SingleChoiceSegmentedButtonRow(
                    Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 16.dp, vertical = 8.dp),
                ) {
                    SegmentedButton(
                        selected = state.status == "active",
                        onClick = { viewModel.setStatus("active") },
                        shape = SegmentedButtonDefaults.itemShape(0, 2),
                    ) { Text(stringResource(R.string.trips_active)) }
                    SegmentedButton(
                        selected = state.status == "past",
                        onClick = { viewModel.setStatus("past") },
                        shape = SegmentedButtonDefaults.itemShape(1, 2),
                    ) { Text(stringResource(R.string.trips_past)) }
                }
            }
        },
    ) { padding ->
        PullToRefreshBox(
            isRefreshing = false,
            onRefresh = viewModel::refresh,
            modifier = Modifier
                .fillMaxSize()
                .padding(padding),
        ) {
            when (val trips = state.trips) {
                is UiState.Loading -> SkeletonList(itemHeight = 190)

                is UiState.Failure -> ErrorView(trips.error, onRetry = viewModel::refresh)

                is UiState.Success -> {
                    if (trips.value.items.isEmpty()) {
                        EmptyView(
                            title = stringResource(R.string.trips_empty),
                            icon = Icons.Rounded.Route,
                        )
                    } else {
                        LazyColumn(
                            contentPadding = PaddingValues(16.dp),
                            verticalArrangement = Arrangement.spacedBy(12.dp),
                            modifier = Modifier.fillMaxSize(),
                        ) {
                            items(trips.value.items, key = { it.id }) { trip ->
                                TripCard(
                                    trip = trip,
                                    fmt = fmt,
                                    tripRepository = tripRepository,
                                    advancing = state.advancingId == trip.id,
                                    onAdvance = { to ->
                                        viewModel.advance(trip, to, genericError)
                                    },
                                )
                            }
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun TripCard(
    trip: Trip,
    fmt: Fmt,
    tripRepository: TripRepository,
    advancing: Boolean,
    onAdvance: (TripStage) -> Unit,
) {
    val scheme = MaterialTheme.colorScheme
    val context = LocalContext.current
    val next = trip.stage.next
    val shipperPhone = trip.company?.phone

    Card(Modifier.fillMaxWidth()) {
        Column(Modifier.padding(14.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    trip.tripNumber,
                    style = MaterialTheme.typography.labelMedium,
                    color = scheme.onSurfaceVariant,
                )
                Spacer(Modifier.weight(1f))
                StatusBadge(
                    stringResource(trip.stage.labelRes),
                    statusColor(trip.stage.wire, scheme),
                )
            }

            Spacer(Modifier.height(8.dp))
            Text(
                trip.load?.title ?: Fmt.EM_DASH,
                style = MaterialTheme.typography.titleMedium,
                fontWeight = FontWeight.SemiBold,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            Spacer(Modifier.height(4.dp))
            Text(
                "${trip.load?.pickupFormatted ?: Fmt.EM_DASH} → " +
                    (trip.load?.deliveryFormatted ?: Fmt.EM_DASH),
                style = MaterialTheme.typography.bodySmall,
                color = scheme.onSurfaceVariant,
            )

            Spacer(Modifier.height(12.dp))
            ProgressRail(trip.stage)

            Spacer(Modifier.height(12.dp))
            Row(verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    Text(
                        fmt.money(trip.driverPayout, trip.currency),
                        style = MaterialTheme.typography.titleMedium,
                        fontWeight = FontWeight.Bold,
                        color = scheme.secondary,
                    )
                    Text(
                        "${stringResource(R.string.trips_commission)} " +
                            fmt.money(trip.commissionAmount, trip.currency),
                        style = MaterialTheme.typography.labelSmall,
                        color = scheme.onSurfaceVariant,
                    )
                }
                if (shipperPhone != null) {
                    FilledTonalIconButton(
                        onClick = {
                            context.startActivity(
                                Intent(Intent.ACTION_DIAL, Uri.parse("tel:$shipperPhone")),
                            )
                        },
                    ) {
                        Icon(
                            Icons.Rounded.Phone,
                            contentDescription = stringResource(R.string.common_call),
                        )
                    }
                }
            }

            if (trip.isTracking) {
                Spacer(Modifier.height(8.dp))
                TrackingBanner(tripRepository)
            }

            if (next != null) {
                Spacer(Modifier.height(12.dp))
                Button(
                    onClick = { onAdvance(next) },
                    enabled = !advancing,
                    modifier = Modifier
                        .fillMaxWidth()
                        .height(48.dp),
                ) {
                    if (advancing) {
                        CircularProgressIndicator(
                            strokeWidth = 2.dp,
                            modifier = Modifier.size(18.dp),
                            color = scheme.onPrimary,
                        )
                    } else {
                        Icon(Icons.AutoMirrored.Rounded.ArrowForward, contentDescription = null)
                        Spacer(Modifier.width(8.dp))
                        Text(
                            stringResource(
                                R.string.trips_next_stage,
                                stringResource(next.labelRes),
                            ),
                        )
                    }
                }
            } else if (trip.stage == TripStage.DELIVERED) {
                // The driver's part is done; only the shipper can close the
                // trip, so no button is offered — an inert one invites taps.
                Spacer(Modifier.height(12.dp))
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Icon(
                        Icons.Rounded.HourglassTop,
                        contentDescription = null,
                        tint = scheme.onSurfaceVariant,
                        modifier = Modifier.size(16.dp),
                    )
                    Spacer(Modifier.width(6.dp))
                    Text(
                        stringResource(R.string.trips_awaiting_confirmation),
                        style = MaterialTheme.typography.bodySmall,
                        color = scheme.onSurfaceVariant,
                    )
                }
            }
        }
    }
}

/**
 * Starts position reporting while an active trip is on screen, asking for the
 * permission the first time it is actually needed rather than at launch.
 */
@Composable
private fun TrackingBanner(tripRepository: TripRepository) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val reporter = remember(context) { LocationReporter(context, tripRepository) }
    var tracking by remember { mutableStateOf(TrackingState.OFF) }

    val permissionLauncher = rememberLauncherForActivityResult(
        ActivityResultContracts.RequestMultiplePermissions(),
    ) { granted ->
        if (granted.values.any { it }) {
            reporter.start(scope) { tracking = it }
        } else {
            tracking = TrackingState.DENIED
        }
    }

    DisposableEffect(Unit) {
        if (reporter.hasPermission()) {
            reporter.start(scope) { tracking = it }
        } else {
            permissionLauncher.launch(
                arrayOf(
                    Manifest.permission.ACCESS_FINE_LOCATION,
                    Manifest.permission.ACCESS_COARSE_LOCATION,
                ),
            )
        }
        onDispose { reporter.stop { tracking = it } }
    }

    val scheme = MaterialTheme.colorScheme
    val (icon, label, tint) = when (tracking) {
        TrackingState.LIVE -> Triple(
            Icons.Rounded.MyLocation,
            stringResource(R.string.trips_tracking_on),
            scheme.secondary,
        )
        TrackingState.DENIED -> Triple(
            Icons.Rounded.LocationDisabled,
            stringResource(R.string.trips_tracking_denied),
            scheme.error,
        )
        TrackingState.OFF -> Triple(
            Icons.Rounded.LocationDisabled,
            stringResource(R.string.trips_tracking_off),
            scheme.onSurfaceVariant,
        )
    }

    Row(verticalAlignment = Alignment.CenterVertically) {
        Icon(icon, contentDescription = null, tint = tint, modifier = Modifier.size(15.dp))
        Spacer(Modifier.width(6.dp))
        Text(label, style = MaterialTheme.typography.labelSmall, color = tint)
        if (tracking == TrackingState.DENIED) {
            Spacer(Modifier.weight(1f))
            TextButton(
                onClick = {
                    context.startActivity(
                        Intent(
                            android.provider.Settings.ACTION_APPLICATION_DETAILS_SETTINGS,
                            Uri.fromParts("package", context.packageName, null),
                        ),
                    )
                },
            ) {
                Text(stringResource(R.string.common_open_settings))
            }
        }
    }
}

/** A dot per stage, filled up to where the trip has reached. */
@Composable
private fun ProgressRail(stage: TripStage) {
    val scheme = MaterialTheme.colorScheme
    val reached = TripStage.DRIVER_FLOW.indexOf(stage)
    val done = stage == TripStage.COMPLETED

    Row(Modifier.fillMaxWidth()) {
        TripStage.DRIVER_FLOW.forEachIndexed { i, _ ->
            val filled = done || (reached >= 0 && i <= reached)
            Box(
                Modifier
                    .weight(1f)
                    .padding(end = if (i == TripStage.DRIVER_FLOW.lastIndex) 0.dp else 3.dp)
                    .height(5.dp)
                    .background(
                        if (filled) scheme.secondary else scheme.surfaceContainerHighest,
                        RoundedCornerShape(3.dp),
                    ),
            )
        }
    }
}
