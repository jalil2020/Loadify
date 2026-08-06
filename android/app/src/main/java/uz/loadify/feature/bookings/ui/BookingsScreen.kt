package uz.loadify.feature.bookings.ui

import android.content.Intent
import android.net.Uri
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.BookmarkBorder
import androidx.compose.material.icons.rounded.Phone
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
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
import uz.loadify.feature.auth.domain.CurrentUser
import uz.loadify.feature.bookings.domain.Booking
import uz.loadify.feature.trips.ui.BookingsViewModel

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun BookingsScreen(
    user: CurrentUser?,
    fmt: Fmt,
    viewModel: BookingsViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val snackbar = remember { SnackbarHostState() }
    val genericError = stringResource(R.string.common_error)

    // A company user signing in on the phone sees the mirror image of the same
    // screen, so the viewer decides which counterparty is shown.
    val asDriver = user?.isDriver != false

    LaunchedEffect(state.error) {
        state.error?.let {
            snackbar.showSnackbar(it)
            viewModel.consumeError()
        }
    }

    Scaffold(
        snackbarHost = { SnackbarHost(snackbar) },
        topBar = { TopAppBar(title = { Text(stringResource(R.string.bookings_title)) }) },
    ) { padding ->
        PullToRefreshBox(
            isRefreshing = false,
            onRefresh = viewModel::refresh,
            modifier = Modifier
                .fillMaxSize()
                .padding(padding),
        ) {
            when (val bookings = state.bookings) {
                is UiState.Loading -> SkeletonList(itemHeight = 160)

                is UiState.Failure -> ErrorView(bookings.error, onRetry = viewModel::refresh)

                is UiState.Success -> {
                    if (bookings.value.items.isEmpty()) {
                        EmptyView(
                            title = stringResource(R.string.bookings_empty),
                            icon = Icons.Rounded.BookmarkBorder,
                        )
                    } else {
                        LazyColumn(
                            contentPadding = PaddingValues(16.dp),
                            verticalArrangement = Arrangement.spacedBy(12.dp),
                            modifier = Modifier.fillMaxSize(),
                        ) {
                            items(bookings.value.items, key = { it.id }) { booking ->
                                BookingCard(
                                    booking = booking,
                                    fmt = fmt,
                                    asDriver = asDriver,
                                    busy = state.busyId == booking.id,
                                    onRespond = { accept ->
                                        viewModel.respond(booking, accept, genericError)
                                    },
                                    onWithdraw = { viewModel.withdraw(booking, genericError) },
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
private fun BookingCard(
    booking: Booking,
    fmt: Fmt,
    asDriver: Boolean,
    busy: Boolean,
    onRespond: (Boolean) -> Unit,
    onWithdraw: () -> Unit,
) {
    val scheme = MaterialTheme.colorScheme
    val context = LocalContext.current
    val phone = booking.counterpartyPhone(asDriver)

    Card(Modifier.fillMaxWidth()) {
        Column(Modifier.padding(14.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    booking.load?.referenceNo ?: Fmt.EM_DASH,
                    style = MaterialTheme.typography.labelMedium,
                    color = scheme.onSurfaceVariant,
                )
                Spacer(Modifier.weight(1f))
                StatusBadge(booking.status, statusColor(booking.status, scheme))
            }

            Spacer(Modifier.height(8.dp))
            Text(
                booking.load?.title ?: Fmt.EM_DASH,
                style = MaterialTheme.typography.titleMedium,
                fontWeight = FontWeight.SemiBold,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            Spacer(Modifier.height(4.dp))
            Text(
                "${booking.load?.pickupFormatted ?: Fmt.EM_DASH} → " +
                    (booking.load?.deliveryFormatted ?: Fmt.EM_DASH),
                style = MaterialTheme.typography.bodySmall,
                color = scheme.onSurfaceVariant,
            )

            Spacer(Modifier.height(10.dp))
            Row(verticalAlignment = Alignment.Bottom) {
                Column(Modifier.weight(1f)) {
                    Text(
                        booking.counterpartyName(asDriver),
                        style = MaterialTheme.typography.bodyMedium,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                    Text(
                        phone?.let { Fmt.phone(it) }
                            ?: stringResource(R.string.board_contact_hidden),
                        style = MaterialTheme.typography.labelSmall,
                        color = scheme.onSurfaceVariant,
                    )
                }
                Text(
                    fmt.money(booking.price, booking.currency),
                    style = MaterialTheme.typography.titleMedium,
                    fontWeight = FontWeight.Bold,
                    color = scheme.secondary,
                )
            }

            if (booking.isPending && booking.expiresAt != null) {
                Spacer(Modifier.height(6.dp))
                Text(
                    fmt.relative(booking.expiresAt),
                    style = MaterialTheme.typography.labelSmall,
                    color = scheme.tertiary,
                )
            }

            if (phone != null) {
                Spacer(Modifier.height(8.dp))
                OutlinedButton(
                    onClick = {
                        context.startActivity(Intent(Intent.ACTION_DIAL, Uri.parse("tel:$phone")))
                    },
                    modifier = Modifier.fillMaxWidth(),
                ) {
                    Icon(Icons.Rounded.Phone, contentDescription = null)
                    Spacer(Modifier.width(8.dp))
                    Text(stringResource(R.string.common_call))
                }
            }

            if (booking.canRespond(asDriver)) {
                Spacer(Modifier.height(12.dp))
                Row {
                    Button(
                        onClick = { onRespond(true) },
                        enabled = !busy,
                        modifier = Modifier
                            .weight(1f)
                            .height(48.dp),
                    ) {
                        Text(stringResource(R.string.bookings_accept))
                    }
                    Spacer(Modifier.width(10.dp))
                    OutlinedButton(
                        onClick = { onRespond(false) },
                        enabled = !busy,
                        modifier = Modifier
                            .weight(1f)
                            .height(48.dp),
                    ) {
                        Text(stringResource(R.string.bookings_reject))
                    }
                }
            } else if (booking.canWithdraw(asDriver)) {
                Spacer(Modifier.height(12.dp))
                OutlinedButton(
                    onClick = onWithdraw,
                    enabled = !busy,
                    modifier = Modifier
                        .fillMaxWidth()
                        .height(48.dp),
                ) {
                    Text(stringResource(R.string.bookings_withdraw))
                }
            }
        }
    }
}
