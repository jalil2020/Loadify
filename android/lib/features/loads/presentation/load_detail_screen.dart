import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../../app/locale.dart';
import '../../../app/providers.dart';
import '../../../app/theme.dart';
import '../../../core/error/api_exception.dart';
import '../../../core/format/formatters.dart';
import '../../../core/widgets/states.dart';
import '../../../l10n/strings.dart';
import '../domain/load.dart';

class LoadDetailScreen extends ConsumerStatefulWidget {
  const LoadDetailScreen({required this.loadId, super.key});

  final String loadId;

  @override
  ConsumerState<LoadDetailScreen> createState() => _LoadDetailScreenState();
}

class _LoadDetailScreenState extends ConsumerState<LoadDetailScreen> {
  bool _reserving = false;

  Future<void> _reserve(Load load, S s) async {
    setState(() => _reserving = true);
    try {
      await ref.read(loadRepositoryProvider).reserve(load.id);
      if (!mounted) return;
      ref
        ..invalidate(loadDetailProvider(load.id))
        ..invalidate(loadsProvider)
        ..invalidate(bookingsProvider);
      ScaffoldMessenger.of(context)
        ..hideCurrentSnackBar()
        ..showSnackBar(
          SnackBar(
            content: Text('${load.referenceNo}: ${s.reserved}'),
            behavior: SnackBarBehavior.floating,
            backgroundColor: Theme.of(context).colorScheme.secondary,
          ),
        );
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
    } finally {
      if (mounted) setState(() => _reserving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final S s = ref.watch(stringsProvider);
    final Fmt fmt = ref.watch(fmtProvider);
    final AsyncValue<Load> load = ref.watch(loadDetailProvider(widget.loadId));
    final ColorScheme scheme = Theme.of(context).colorScheme;
    final bool canReserve =
        ref.watch(authProvider)?.can('bookings.create') ?? false;

    return Scaffold(
      appBar: AppBar(title: Text(load.valueOrNull?.referenceNo ?? '')),
      body: load.when(
        loading: () => const SkeletonList(count: 3, height: 120),
        error: (Object e, _) => ErrorView(
          error: e,
          s: s,
          onRetry: () => ref.invalidate(loadDetailProvider(widget.loadId)),
        ),
        data: (Load l) => ListView(
          padding: const EdgeInsets.fromLTRB(16, 12, 16, 100),
          children: <Widget>[
            Row(
              children: <Widget>[
                Expanded(
                  child: Text(
                    l.title,
                    style: Theme.of(context).textTheme.headlineSmall?.copyWith(
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                ),
                StatusBadge(
                  label: l.status,
                  color: LoadifyTheme.statusColor(l.status, scheme),
                ),
              ],
            ),
            if (l.description != null && l.description!.isNotEmpty) ...<Widget>[
              const SizedBox(height: 6),
              Text(
                l.description!,
                style: Theme.of(context).textTheme.bodyMedium,
              ),
            ],
            const SizedBox(height: 16),

            _Section(
              title: s.route,
              children: <Widget>[
                _Row(label: s.from, value: l.pickupAddress?.formatted ?? '—'),
                _Row(label: s.to, value: l.deliveryAddress?.formatted ?? '—'),
                _Row(label: s.pickupDate, value: fmt.date(l.pickupFrom)),
                if (l.distanceKm != null)
                  _Row(label: 'km', value: fmt.distance(l.distanceKm)),
              ],
            ),
            const SizedBox(height: 12),

            _Section(
              title: s.cargo,
              children: <Widget>[
                _Row(label: s.weight, value: fmt.weight(l.weightKg)),
                if (l.requiredVehicleType != null)
                  _Row(label: s.vehicleType, value: l.requiredVehicleType!),
                if (l.requiresAdr) _Row(label: s.requirements, value: s.adr),
                if (l.requiresRefrigeration)
                  _Row(label: s.requirements, value: s.refrigerated),
              ],
            ),
            const SizedBox(height: 12),

            _Section(
              title: s.shipper,
              children: <Widget>[
                _Row(label: '', value: l.company?.brandName ?? '—'),
                _Row(
                  label: s.rating,
                  value: '★ ${(l.company?.rating ?? 0).toStringAsFixed(1)}',
                ),
                // Masked until a booking exists. Shown as an explicit state so
                // the driver knows a number exists to be earned.
                _Row(
                  label: s.call,
                  value: l.company?.phone == null
                      ? s.contactHidden
                      : Fmt.phone(l.company?.phone),
                ),
              ],
            ),

            if (l.company?.phone != null) ...<Widget>[
              const SizedBox(height: 12),
              OutlinedButton.icon(
                onPressed: () => unawaited(
                  launchUrl(Uri(scheme: 'tel', path: l.company!.phone)),
                ),
                icon: const Icon(Icons.phone_rounded),
                label: Text(s.call),
              ),
            ],
          ],
        ),
      ),
      bottomNavigationBar: load.valueOrNull == null
          ? null
          : SafeArea(
              child: Padding(
                padding: const EdgeInsets.all(16),
                child: Row(
                  children: <Widget>[
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        mainAxisSize: MainAxisSize.min,
                        children: <Widget>[
                          Text(
                            fmt.money(
                              load.value!.priceAmount,
                              load.value!.priceCurrency,
                            ),
                            style: Theme.of(context).textTheme.titleLarge
                                ?.copyWith(
                                  fontWeight: FontWeight.w700,
                                  color: scheme.secondary,
                                ),
                          ),
                          if (load.value!.pricePerKm != null)
                            Text(
                              '${fmt.money(load.value!.pricePerKm!, load.value!.priceCurrency)} ${s.perKm}',
                              style: Theme.of(context).textTheme.labelSmall,
                            ),
                        ],
                      ),
                    ),
                    const SizedBox(width: 12),
                    FilledButton(
                      onPressed:
                          (!canReserve || load.value!.isBooked || _reserving)
                          ? null
                          : () => _reserve(load.value!, s),
                      child: _reserving
                          ? const SizedBox(
                              width: 18,
                              height: 18,
                              child: CircularProgressIndicator(strokeWidth: 2),
                            )
                          : Text(load.value!.isBooked ? s.reserved : s.reserve),
                    ),
                  ],
                ),
              ),
            ),
    );
  }
}

class _Section extends StatelessWidget {
  const _Section({required this.title, required this.children});

  final String title;
  final List<Widget> children;

  @override
  Widget build(BuildContext context) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(14),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: <Widget>[
            Text(title, style: Theme.of(context).textTheme.titleSmall),
            const SizedBox(height: 8),
            ...children,
          ],
        ),
      ),
    );
  }
}

class _Row extends StatelessWidget {
  const _Row({required this.label, required this.value});

  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    final ColorScheme scheme = Theme.of(context).colorScheme;
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 4),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: <Widget>[
          SizedBox(
            width: 96,
            child: Text(
              label,
              style: Theme.of(
                context,
              ).textTheme.bodySmall?.copyWith(color: scheme.onSurfaceVariant),
            ),
          ),
          Expanded(
            child: Text(value, style: Theme.of(context).textTheme.bodyMedium),
          ),
        ],
      ),
    );
  }
}
