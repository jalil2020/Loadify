package uz.loadify.core.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.CloudOff
import androidx.compose.material.icons.rounded.Inbox
import androidx.compose.material.icons.rounded.Refresh
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import uz.loadify.R
import uz.loadify.core.error.ApiException

/**
 * The three non-happy states every list screen needs.
 *
 * Centralised because a driver on a bad connection sees these more often than
 * they see data, and an inconsistent error screen reads as an unreliable app.
 */
@Composable
fun ErrorView(
    error: Throwable,
    onRetry: (() -> Unit)? = null,
    modifier: Modifier = Modifier,
) {
    val api = error as? ApiException
    // A 5xx or a dropped connection is worth retrying; a 422 is not, and
    // offering a button that will fail again is a small lie.
    val retryable = api == null || api.isTransient

    Box(modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Column(
            horizontalAlignment = Alignment.CenterHorizontally,
            modifier = Modifier.padding(24.dp),
        ) {
            Icon(
                Icons.Rounded.CloudOff,
                contentDescription = null,
                tint = MaterialTheme.colorScheme.outline,
                modifier = Modifier.height(44.dp),
            )
            Spacer(Modifier.height(12.dp))
            Text(
                text = api?.message ?: stringResource(R.string.common_error),
                textAlign = TextAlign.Center,
                style = MaterialTheme.typography.bodyMedium,
            )
            api?.traceId?.let {
                Spacer(Modifier.height(6.dp))
                Text(
                    text = it,
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.outline,
                )
            }
            if (onRetry != null && retryable) {
                Spacer(Modifier.height(16.dp))
                OutlinedButton(onClick = onRetry) {
                    Icon(Icons.Rounded.Refresh, contentDescription = null)
                    Spacer(Modifier.height(0.dp))
                    Text(stringResource(R.string.common_retry), Modifier.padding(start = 8.dp))
                }
            }
        }
    }
}

@Composable
fun EmptyView(
    title: String,
    hint: String? = null,
    icon: ImageVector = Icons.Rounded.Inbox,
    modifier: Modifier = Modifier,
) {
    Box(modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Column(
            horizontalAlignment = Alignment.CenterHorizontally,
            modifier = Modifier.padding(24.dp),
        ) {
            Icon(
                icon,
                contentDescription = null,
                tint = MaterialTheme.colorScheme.outline,
                modifier = Modifier.height(44.dp),
            )
            Spacer(Modifier.height(12.dp))
            Text(title, style = MaterialTheme.typography.titleMedium)
            if (hint != null) {
                Spacer(Modifier.height(4.dp))
                Text(
                    hint,
                    textAlign = TextAlign.Center,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

/**
 * A shimmer-free skeleton. Plain blocks read as "loading" just as well and cost
 * nothing to animate on the low-end devices this app targets.
 */
@Composable
fun SkeletonList(
    count: Int = 5,
    itemHeight: Int = 130,
    contentPadding: PaddingValues = PaddingValues(16.dp),
) {
    LazyColumn(
        contentPadding = contentPadding,
        verticalArrangement = Arrangement.spacedBy(12.dp),
        modifier = Modifier.fillMaxSize(),
    ) {
        items(List(count) { it }) {
            Box(
                Modifier
                    .fillMaxWidth()
                    .height(itemHeight.dp)
                    .background(
                        MaterialTheme.colorScheme.surfaceContainerHighest,
                        RoundedCornerShape(14.dp),
                    ),
            )
        }
    }
}

@Composable
fun StatusBadge(label: String, color: Color, modifier: Modifier = Modifier) {
    Box(
        modifier
            .background(color.copy(alpha = 0.12f), RoundedCornerShape(6.dp))
            .padding(horizontal = 8.dp, vertical = 3.dp),
    ) {
        Text(
            label,
            style = MaterialTheme.typography.labelSmall,
            color = color,
        )
    }
}
