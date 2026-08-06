import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:geolocator/geolocator.dart';

import '../../../app/providers.dart';
import '../../../core/config/env.dart';
import '../../../l10n/strings.dart';
import '../domain/trip.dart';

/// Why a trip is or is not sharing position.
enum TrackingState { off, denied, disabled, live }

/// Reports the driver's position while a trip is moving.
///
/// Deliberately foreground-only. Background location on Android requires a
/// separate, prominently-disclosed permission and a Play Store justification;
/// shipping that before it is actually needed would risk the listing. A driver
/// with the app open — which is the normal case in a cab on a mount — gets live
/// tracking, and the server falls back to the last known point otherwise.
class LocationReporter extends StateNotifier<TrackingState> {
  LocationReporter(this._ref) : super(TrackingState.off);

  final Ref _ref;
  Timer? _timer;
  bool _sending = false;

  Future<void> start() async {
    if (_timer != null) return;

    if (!await Geolocator.isLocationServiceEnabled()) {
      state = TrackingState.disabled;
      return;
    }

    LocationPermission permission = await Geolocator.checkPermission();
    if (permission == LocationPermission.denied) {
      permission = await Geolocator.requestPermission();
    }
    if (permission == LocationPermission.denied ||
        permission == LocationPermission.deniedForever) {
      state = TrackingState.denied;
      return;
    }

    state = TrackingState.live;
    // An immediate first fix, then on a cadence: a driver who just tapped
    // "en route" expects the shipper to see them move now, not in 30 seconds.
    unawaited(_report());
    _timer = Timer.periodic(Env.locationInterval, (_) => unawaited(_report()));
  }

  Future<void> _report() async {
    // A slow fix must not queue up behind the next tick, or a tunnel produces
    // a burst of stale positions on the way out.
    if (_sending) return;
    _sending = true;
    try {
      final Position position = await Geolocator.getCurrentPosition(
        locationSettings: const LocationSettings(
          accuracy: LocationAccuracy.high,
          distanceFilter: 50,
        ),
      );
      await _ref
          .read(tripRepositoryProvider)
          .reportLocation(
            lat: position.latitude,
            lng: position.longitude,
            speedKph: position.speed * 3.6,
            headingDeg: position.heading,
          );
    } on Object {
      // A dropped ping is not worth interrupting a driver for; the next one is
      // thirty seconds away and the server keeps the last known point.
    } finally {
      _sending = false;
    }
  }

  void stop() {
    _timer?.cancel();
    _timer = null;
    state = TrackingState.off;
  }

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }
}

final StateNotifierProvider<LocationReporter, TrackingState>
locationReporterProvider =
    StateNotifierProvider<LocationReporter, TrackingState>(
      (Ref ref) => LocationReporter(ref),
    );

/// Shows tracking state on an active trip and starts the reporter while the
/// card is on screen.
class LocationReporterBanner extends ConsumerStatefulWidget {
  const LocationReporterBanner({
    required this.trip,
    required this.s,
    super.key,
  });

  final Trip trip;
  final S s;

  @override
  ConsumerState<LocationReporterBanner> createState() =>
      _LocationReporterBannerState();
}

class _LocationReporterBannerState
    extends ConsumerState<LocationReporterBanner> {
  @override
  void initState() {
    super.initState();
    // Started after the first frame: requesting a permission during build
    // would run a platform channel call inside the widget lifecycle.
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted && widget.trip.isTracking) {
        unawaited(ref.read(locationReporterProvider.notifier).start());
      }
    });
  }

  @override
  Widget build(BuildContext context) {
    final TrackingState state = ref.watch(locationReporterProvider);
    final ColorScheme scheme = Theme.of(context).colorScheme;

    final (IconData icon, String label, Color color) = switch (state) {
      TrackingState.live => (
        Icons.my_location_rounded,
        widget.s.tracking,
        scheme.secondary,
      ),
      TrackingState.denied => (
        Icons.location_disabled_rounded,
        widget.s.trackingOff,
        scheme.error,
      ),
      TrackingState.disabled => (
        Icons.location_off_rounded,
        widget.s.trackingOff,
        scheme.error,
      ),
      TrackingState.off => (
        Icons.location_searching_rounded,
        widget.s.loading,
        scheme.onSurfaceVariant,
      ),
    };

    return Row(
      children: <Widget>[
        Icon(icon, size: 15, color: color),
        const SizedBox(width: 6),
        Text(
          label,
          style: Theme.of(context).textTheme.labelSmall?.copyWith(color: color),
        ),
        if (state == TrackingState.denied ||
            state == TrackingState.disabled) ...<Widget>[
          const Spacer(),
          TextButton(
            onPressed: () => unawaited(Geolocator.openAppSettings()),
            child: Text(widget.s.retry),
          ),
        ],
      ],
    );
  }
}
