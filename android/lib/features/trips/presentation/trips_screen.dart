import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../../app/locale.dart';
import '../../../app/providers.dart';
import '../../../app/theme.dart';
import '../../../core/error/api_exception.dart';
import '../../../core/format/formatters.dart';
import '../../../core/network/api_client.dart';
import '../../../core/widgets/states.dart';
import '../../../l10n/strings.dart';
import '../domain/trip.dart';
import 'location_reporter.dart';

class TripsScreen extends ConsumerStatefulWidget {
  const TripsScreen({super.key});

  @override
  ConsumerState<TripsScreen> createState() => _TripsScreenState();
}

class _TripsScreenState extends ConsumerState<TripsScreen> {
  String _status = 'active';
  String? _advancing;

  Future<void> _advance(Trip trip, TripStage to, S s) async {
    setState(() => _advancing = trip.id);
    try {
      await ref.read(tripRepositoryProvider).advance(trip.id, to);
      if (!mounted) return;
      ref.invalidate(tripsProvider);
    } on ApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context)
        ..hideCurrentSnackBar()
        ..showSnackBar(
          SnackBar(
            content: Text(e.message),
            behavior: SnackBarBehavior.floating,
            backgroundColor: Theme.of(context).colorScheme.error,
          ),
        );
    } on SessionExpired {
      // The router redirects on sign-out; nothing to show here.
    } finally {
      if (mounted) setState(() => _advancing = null);
    }
  }

  @override
  Widget build(BuildContext context) {
    final S s = ref.watch(stringsProvider);
    final Fmt fmt = ref.watch(fmtProvider);
    final AsyncValue<Paged<Trip>> trips = ref.watch(tripsProvider(_status));

    return Scaffold(
      appBar: AppBar(
        title: Text(s.tripsTitle),
        bottom: PreferredSize(
          preferredSize: const Size.fromHeight(52),
          child: Padding(
            padding: const EdgeInsets.fromLTRB(16, 0, 16, 10),
            child: SegmentedButton<String>(
              segments: <ButtonSegment<String>>[
                ButtonSegment<String>(
                  value: 'active',
                  label: Text(s.activeTrips),
                ),
                ButtonSegment<String>(value: 'past', label: Text(s.pastTrips)),
              ],
              selected: <String>{_status},
              onSelectionChanged: (Set<String> v) =>
                  setState(() => _status = v.first),
            ),
          ),
        ),
      ),
      body: RefreshIndicator(
        onRefresh: () async => ref.invalidate(tripsProvider),
        child: trips.when(
          loading: () => const SkeletonList(height: 190),
          error: (Object e, _) => ErrorView(
            error: e,
            s: s,
            onRetry: () => ref.invalidate(tripsProvider),
          ),
          data: (Paged<Trip> page) {
            if (page.items.isEmpty) {
              return ListView(
                physics: const AlwaysScrollableScrollPhysics(),
                children: <Widget>[
                  SizedBox(
                    height: MediaQuery.sizeOf(context).height * 0.6,
                    child: EmptyView(
                      title: s.noTrips,
                      icon: Icons.route_outlined,
                    ),
                  ),
                ],
              );
            }
            return ListView.separated(
              padding: const EdgeInsets.fromLTRB(16, 12, 16, 24),
              physics: const AlwaysScrollableScrollPhysics(),
              itemCount: page.items.length,
              separatorBuilder: (_, _) => const SizedBox(height: 12),
              itemBuilder: (BuildContext context, int i) {
                final Trip trip = page.items[i];
                return _TripCard(
                  trip: trip,
                  s: s,
                  fmt: fmt,
                  advancing: _advancing == trip.id,
                  onAdvance: (TripStage to) => _advance(trip, to, s),
                );
              },
            );
          },
        ),
      ),
    );
  }
}

class _TripCard extends ConsumerWidget {
  const _TripCard({
    required this.trip,
    required this.s,
    required this.fmt,
    required this.advancing,
    required this.onAdvance,
  });

  final Trip trip;
  final S s;
  final Fmt fmt;
  final bool advancing;
  final ValueChanged<TripStage> onAdvance;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final ColorScheme scheme = Theme.of(context).colorScheme;
    final TextTheme text = Theme.of(context).textTheme;
    final TripStage? next = trip.stage.next;
    final String? shipperPhone = trip.company?.phone;

    return Card(
      child: Padding(
        padding: const EdgeInsets.all(14),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: <Widget>[
            Row(
              children: <Widget>[
                Text(
                  trip.tripNumber,
                  style: text.labelMedium?.copyWith(
                    color: scheme.onSurfaceVariant,
                  ),
                ),
                const Spacer(),
                StatusBadge(
                  label: s.stage(trip.stage.wire),
                  color: LoadifyTheme.statusColor(trip.stage.wire, scheme),
                ),
              ],
            ),
            const SizedBox(height: 8),
            Text(
              trip.load?.title ?? '—',
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: text.titleMedium?.copyWith(fontWeight: FontWeight.w600),
            ),
            const SizedBox(height: 4),
            Text(
              '${trip.load?.pickupFormatted ?? '—'} → ${trip.load?.deliveryFormatted ?? '—'}',
              style: text.bodySmall?.copyWith(color: scheme.onSurfaceVariant),
            ),
            const SizedBox(height: 12),
            _ProgressRail(stage: trip.stage, scheme: scheme),
            const SizedBox(height: 12),
            Row(
              children: <Widget>[
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: <Widget>[
                      Text(
                        fmt.money(trip.driverPayout, trip.currency),
                        style: text.titleMedium?.copyWith(
                          fontWeight: FontWeight.w700,
                          color: scheme.secondary,
                        ),
                      ),
                      Text(
                        '${s.commission} ${fmt.money(trip.commissionAmount, trip.currency)}',
                        style: text.labelSmall?.copyWith(
                          color: scheme.onSurfaceVariant,
                        ),
                      ),
                    ],
                  ),
                ),
                if (shipperPhone != null)
                  IconButton.filledTonal(
                    tooltip: s.call,
                    onPressed: () => unawaited(
                      launchUrl(Uri(scheme: 'tel', path: shipperPhone)),
                    ),
                    icon: const Icon(Icons.phone_rounded),
                  ),
              ],
            ),
            if (trip.isTracking) ...<Widget>[
              const SizedBox(height: 8),
              LocationReporterBanner(trip: trip, s: s),
            ],
            if (next != null) ...<Widget>[
              const SizedBox(height: 12),
              FilledButton.icon(
                onPressed: advancing ? null : () => onAdvance(next),
                icon: advancing
                    ? const SizedBox(
                        width: 16,
                        height: 16,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      )
                    : const Icon(Icons.arrow_forward_rounded),
                label: Text('${s.nextStage}: ${s.stage(next.wire)}'),
              ),
            ] else if (trip.stage == TripStage.delivered) ...<Widget>[
              const SizedBox(height: 12),
              // The driver's part is done; only the shipper can close the trip,
              // so no button is offered — an inert one would just invite taps.
              Row(
                children: <Widget>[
                  Icon(
                    Icons.hourglass_top_rounded,
                    size: 16,
                    color: scheme.onSurfaceVariant,
                  ),
                  const SizedBox(width: 6),
                  Expanded(
                    child: Text(
                      s.awaitingConfirmation,
                      style: text.bodySmall?.copyWith(
                        color: scheme.onSurfaceVariant,
                      ),
                    ),
                  ),
                ],
              ),
            ],
          ],
        ),
      ),
    );
  }
}

/// A dot per stage, filled up to where the trip has reached.
class _ProgressRail extends StatelessWidget {
  const _ProgressRail({required this.stage, required this.scheme});

  final TripStage stage;
  final ColorScheme scheme;

  @override
  Widget build(BuildContext context) {
    final int reached = TripStage.driverFlow.indexOf(stage);
    final bool done = stage == TripStage.completed;

    return Row(
      children: List<Widget>.generate(TripStage.driverFlow.length, (int i) {
        final bool filled = done || (reached >= 0 && i <= reached);
        return Expanded(
          child: Padding(
            padding: EdgeInsets.only(
              right: i == TripStage.driverFlow.length - 1 ? 0 : 3,
            ),
            child: Container(
              height: 5,
              decoration: BoxDecoration(
                color: filled
                    ? scheme.secondary
                    : scheme.surfaceContainerHighest,
                borderRadius: BorderRadius.circular(3),
              ),
            ),
          ),
        );
      }),
    );
  }
}
