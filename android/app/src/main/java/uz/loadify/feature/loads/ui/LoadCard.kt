package uz.loadify.feature.loads.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.material.icons.Icons
// AutoMirrored: the corridor arrow points from pickup to delivery, so it has to
// flip with the layout direction rather than always pointing right.
import androidx.compose.material.icons.automirrored.rounded.ArrowForward
import androidx.compose.material.icons.rounded.Verified
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import uz.loadify.R
import uz.loadify.core.format.Fmt
import uz.loadify.core.ui.StatusBadge
import uz.loadify.core.ui.statusColor
import uz.loadify.feature.loads.domain.Load

/**
 * One row of the board.
 *
 * The corridor and the price lead, because that is what a driver scans for.
 * Masked contact details are shown as an explicit "visible after booking"
 * rather than hidden, so the driver knows there is a number to be earned.
 */
@Composable
fun LoadCard(
    load: Load,
    fmt: Fmt,
    onClick: () -> Unit,
    onReserve: (() -> Unit)?,
    reserving: Boolean,
    modifier: Modifier = Modifier,
) {
    val scheme = MaterialTheme.colorScheme

    Card(onClick = onClick, modifier = modifier.fillMaxWidth()) {
        Column(Modifier.padding(14.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    load.referenceNo,
                    style = MaterialTheme.typography.labelMedium,
                    color = scheme.onSurfaceVariant,
                )
                if (load.isUrgent) {
                    Spacer(Modifier.width(8.dp))
                    StatusBadge("!", scheme.error)
                }
                Spacer(Modifier.weight(1f))
                StatusBadge(load.status, statusColor(load.status, scheme))
            }

            Spacer(Modifier.height(8.dp))
            Text(
                load.title,
                style = MaterialTheme.typography.titleMedium,
                fontWeight = FontWeight.SemiBold,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )

            Spacer(Modifier.height(10.dp))
            Corridor(load, fmt)

            Spacer(Modifier.height(10.dp))
            Row(verticalAlignment = Alignment.Bottom) {
                Column(Modifier.weight(1f)) {
                    Text(
                        fmt.money(load.priceAmount, load.priceCurrency),
                        style = MaterialTheme.typography.titleLarge,
                        fontWeight = FontWeight.Bold,
                        color = scheme.secondary,
                    )
                    load.pricePerKm?.let {
                        Text(
                            "${fmt.money(it, load.priceCurrency)} ${stringResource(R.string.board_per_km)}",
                            style = MaterialTheme.typography.labelSmall,
                            color = scheme.onSurfaceVariant,
                        )
                    }
                }
                if (onReserve != null) {
                    Button(
                        onClick = onReserve,
                        enabled = !load.isBooked && !reserving,
                        modifier = Modifier.height(44.dp),
                    ) {
                        if (reserving) {
                            CircularProgressIndicator(
                                strokeWidth = 2.dp,
                                modifier = Modifier.size(18.dp),
                                color = scheme.onPrimary,
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

            Spacer(Modifier.height(8.dp))
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    load.company?.brandName ?: Fmt.EM_DASH,
                    style = MaterialTheme.typography.bodySmall,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f),
                )
                if (load.company?.isVerified == true) {
                    Icon(
                        Icons.Rounded.Verified,
                        contentDescription = null,
                        tint = scheme.secondary,
                        modifier = Modifier.size(15.dp),
                    )
                }
                Spacer(Modifier.width(8.dp))
                Text(
                    load.company?.phone?.let { Fmt.phone(it) }
                        ?: stringResource(R.string.board_contact_hidden),
                    style = MaterialTheme.typography.labelSmall,
                    color = scheme.onSurfaceVariant,
                )
            }
        }
    }
}

/** Pickup to delivery, with the date and weight tucked underneath each end. */
@Composable
private fun Corridor(load: Load, fmt: Fmt) {
    val scheme = MaterialTheme.colorScheme

    Row(verticalAlignment = Alignment.Top) {
        Column(Modifier.weight(1f)) {
            Text(
                load.pickupAddress?.shortLabel ?: Fmt.EM_DASH,
                style = MaterialTheme.typography.bodyMedium,
                fontWeight = FontWeight.SemiBold,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            Text(
                fmt.date(load.pickupFrom),
                style = MaterialTheme.typography.labelSmall,
                color = scheme.onSurfaceVariant,
            )
        }

        Column(
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.Center,
            modifier = Modifier.padding(horizontal = 8.dp),
        ) {
            Icon(
                Icons.AutoMirrored.Rounded.ArrowForward,
                contentDescription = null,
                tint = scheme.outline,
                modifier = Modifier.size(16.dp),
            )
            Text(
                fmt.distance(load.distanceKm),
                style = MaterialTheme.typography.labelSmall,
                color = scheme.outline,
            )
        }

        Column(
            horizontalAlignment = Alignment.End,
            modifier = Modifier.weight(1f),
        ) {
            Text(
                load.deliveryAddress?.shortLabel ?: Fmt.EM_DASH,
                style = MaterialTheme.typography.bodyMedium,
                fontWeight = FontWeight.SemiBold,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                textAlign = TextAlign.End,
            )
            Text(
                fmt.weight(load.weightKg),
                style = MaterialTheme.typography.labelSmall,
                color = scheme.onSurfaceVariant,
            )
        }
    }
}
