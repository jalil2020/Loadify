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
import '../domain/booking.dart';

class BookingsScreen extends ConsumerStatefulWidget {
  const BookingsScreen({super.key});

  @override
  ConsumerState<BookingsScreen> createState() => _BookingsScreenState();
}

class _BookingsScreenState extends ConsumerState<BookingsScreen> {
  String? _busyId;

  Future<void> _run(String id, Future<void> Function() action) async {
    setState(() => _busyId = id);
    try {
      await action();
      if (!mounted) return;
      ref.invalidate(bookingsProvider);
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
      // Handled by the router redirect.
    } finally {
      if (mounted) setState(() => _busyId = null);
    }
  }

  @override
  Widget build(BuildContext context) {
    final S s = ref.watch(stringsProvider);
    final Fmt fmt = ref.watch(fmtProvider);
    final AsyncValue<Paged<Booking>> bookings = ref.watch(bookingsProvider);
    final bool asDriver = ref.watch(authProvider)?.isDriver ?? true;

    return Scaffold(
      appBar: AppBar(title: Text(s.bookingsTitle)),
      body: RefreshIndicator(
        onRefresh: () async => ref.invalidate(bookingsProvider),
        child: bookings.when(
          loading: () => const SkeletonList(height: 160),
          error: (Object e, _) => ErrorView(
            error: e,
            s: s,
            onRetry: () => ref.invalidate(bookingsProvider),
          ),
          data: (Paged<Booking> page) {
            if (page.items.isEmpty) {
              return ListView(
                physics: const AlwaysScrollableScrollPhysics(),
                children: <Widget>[
                  SizedBox(
                    height: MediaQuery.sizeOf(context).height * 0.6,
                    child: EmptyView(
                      title: s.noBookings,
                      icon: Icons.bookmark_border_rounded,
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
                final Booking booking = page.items[i];
                return _BookingCard(
                  booking: booking,
                  s: s,
                  fmt: fmt,
                  asDriver: asDriver,
                  busy: _busyId == booking.id,
                  onRespond: (bool accept) => _run(
                    booking.id,
                    () => ref
                        .read(bookingRepositoryProvider)
                        .respond(booking.id, accept: accept),
                  ),
                  onWithdraw: () => _run(
                    booking.id,
                    () => ref
                        .read(bookingRepositoryProvider)
                        .withdraw(booking.id),
                  ),
                );
              },
            );
          },
        ),
      ),
    );
  }
}

class _BookingCard extends StatelessWidget {
  const _BookingCard({
    required this.booking,
    required this.s,
    required this.fmt,
    required this.asDriver,
    required this.busy,
    required this.onRespond,
    required this.onWithdraw,
  });

  final Booking booking;
  final S s;
  final Fmt fmt;
  final bool asDriver;
  final bool busy;
  final ValueChanged<bool> onRespond;
  final VoidCallback onWithdraw;

  @override
  Widget build(BuildContext context) {
    final ColorScheme scheme = Theme.of(context).colorScheme;
    final TextTheme text = Theme.of(context).textTheme;
    final String? phone = booking.counterpartyPhone(asDriver: asDriver);

    return Card(
      child: Padding(
        padding: const EdgeInsets.all(14),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: <Widget>[
            Row(
              children: <Widget>[
                Text(
                  booking.load?.referenceNo ?? '—',
                  style: text.labelMedium?.copyWith(
                    color: scheme.onSurfaceVariant,
                  ),
                ),
                const Spacer(),
                StatusBadge(
                  label: booking.status,
                  color: LoadifyTheme.statusColor(booking.status, scheme),
                ),
              ],
            ),
            const SizedBox(height: 8),
            Text(
              booking.load?.title ?? '—',
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: text.titleMedium?.copyWith(fontWeight: FontWeight.w600),
            ),
            const SizedBox(height: 4),
            Text(
              '${booking.load?.pickupFormatted ?? '—'} → ${booking.load?.deliveryFormatted ?? '—'}',
              style: text.bodySmall?.copyWith(color: scheme.onSurfaceVariant),
            ),
            const SizedBox(height: 10),
            Row(
              children: <Widget>[
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: <Widget>[
                      Text(
                        booking.counterpartyName(asDriver: asDriver),
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: text.bodyMedium,
                      ),
                      Text(
                        phone == null ? s.contactHidden : Fmt.phone(phone),
                        style: text.labelSmall?.copyWith(
                          color: scheme.onSurfaceVariant,
                        ),
                      ),
                    ],
                  ),
                ),
                Text(
                  fmt.money(booking.price, booking.currency),
                  style: text.titleMedium?.copyWith(
                    fontWeight: FontWeight.w700,
                    color: scheme.secondary,
                  ),
                ),
              ],
            ),
            if (booking.expiresAt != null && booking.isPending) ...<Widget>[
              const SizedBox(height: 6),
              Text(
                fmt.relative(booking.expiresAt, s.relative),
                style: text.labelSmall?.copyWith(color: scheme.tertiary),
              ),
            ],
            if (phone != null) ...<Widget>[
              const SizedBox(height: 8),
              OutlinedButton.icon(
                onPressed: () =>
                    unawaited(launchUrl(Uri(scheme: 'tel', path: phone))),
                icon: const Icon(Icons.phone_rounded, size: 18),
                label: Text(s.call),
              ),
            ],
            if (booking.canRespond(asDriver: asDriver)) ...<Widget>[
              const SizedBox(height: 12),
              Row(
                children: <Widget>[
                  Expanded(
                    child: FilledButton(
                      onPressed: busy ? null : () => onRespond(true),
                      child: Text(s.accept),
                    ),
                  ),
                  const SizedBox(width: 10),
                  Expanded(
                    child: OutlinedButton(
                      onPressed: busy ? null : () => onRespond(false),
                      child: Text(s.reject),
                    ),
                  ),
                ],
              ),
            ] else if (booking.canWithdraw(asDriver: asDriver)) ...<Widget>[
              const SizedBox(height: 12),
              OutlinedButton(
                onPressed: busy ? null : onWithdraw,
                child: Text(s.withdraw),
              ),
            ],
          ],
        ),
      ),
    );
  }
}
