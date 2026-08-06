package uz.loadify.feature.trips.ui

import android.Manifest
import android.annotation.SuppressLint
import android.content.Context
import android.content.pm.PackageManager
import androidx.core.content.ContextCompat
import com.google.android.gms.location.LocationServices
import com.google.android.gms.location.Priority
import com.google.android.gms.tasks.CancellationTokenSource
import com.google.android.gms.tasks.Task
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.suspendCancellableCoroutine
import uz.loadify.feature.trips.data.TripRepository
import kotlin.coroutines.resume
import kotlin.time.Duration.Companion.seconds

/** Why a trip is or is not sharing position. */
enum class TrackingState { OFF, DENIED, LIVE }

/**
 * Reports the driver's position while a trip is moving.
 *
 * Deliberately foreground-only. Background location on Android needs a
 * separately-reviewed permission and a prominent-disclosure flow; shipping that
 * before it is actually needed would risk the Play listing. A driver with the
 * app open — the normal case, phone on a windscreen mount — gets live tracking,
 * and the server falls back to the last known point otherwise.
 */
class LocationReporter(
    private val context: Context,
    private val trips: TripRepository,
) {
    private var job: Job? = null

    fun hasPermission(): Boolean =
        ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_FINE_LOCATION) ==
            PackageManager.PERMISSION_GRANTED ||
            ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_COARSE_LOCATION) ==
            PackageManager.PERMISSION_GRANTED

    /**
     * Starts the loop. Idempotent: calling it again while running is a no-op,
     * so a recomposition cannot spawn a second reporter.
     */
    fun start(scope: CoroutineScope, onState: (TrackingState) -> Unit) {
        if (job?.isActive == true) return
        if (!hasPermission()) {
            onState(TrackingState.DENIED)
            return
        }

        onState(TrackingState.LIVE)
        job = scope.launch {
            while (true) {
                report()
                // An immediate first fix, then on a cadence: a driver who just
                // tapped "en route" expects the shipper to see them move now.
                // Thirty seconds after that is a compromise — the shipper sees
                // meaningful movement, and a driver on a rural data plan is not
                // paying for a ping every second.
                delay(INTERVAL)
            }
        }
    }

    fun stop(onState: (TrackingState) -> Unit) {
        job?.cancel()
        job = null
        onState(TrackingState.OFF)
    }

    @SuppressLint("MissingPermission") // Checked in hasPermission before start.
    private suspend fun report() {
        try {
            val client = LocationServices.getFusedLocationProviderClient(context)
            val cancellation = CancellationTokenSource()
            val location = client
                .getCurrentLocation(Priority.PRIORITY_HIGH_ACCURACY, cancellation.token)
                .awaitOrNull() ?: return

            trips.reportLocation(
                lat = location.latitude,
                lng = location.longitude,
                speedKph = (location.speed * 3.6).toDouble(),
                headingDeg = location.bearing.toDouble(),
            )
        } catch (_: Exception) {
            // A dropped ping is not worth interrupting a driver for; the next
            // one is thirty seconds away and the server keeps the last point.
        }
    }

    private companion object {
        val INTERVAL = 30.seconds
    }
}

/**
 * Bridges a Play Services Task to a coroutine.
 *
 * Written here rather than adding kotlinx-coroutines-play-services for one call
 * site. A failed fix resumes null rather than throwing: a missing position is
 * an expected outcome in a tunnel, not an error worth unwinding for.
 */
private suspend fun <T> Task<T>.awaitOrNull(): T? =
    suspendCancellableCoroutine { cont ->
        addOnCompleteListener { task ->
            cont.resume(if (task.isSuccessful) task.result else null)
        }
    }
