import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../app/locale.dart';
import '../../../app/providers.dart';
import '../../../core/error/api_exception.dart';
import '../../../core/format/formatters.dart';
import '../../../core/network/api_client.dart';
import '../../../core/widgets/states.dart';
import '../../../l10n/strings.dart';
import '../../auth/domain/current_user.dart';
import '../../driver/domain/driver_profile.dart';
import '../domain/load.dart';
import 'load_card.dart';

class BoardScreen extends ConsumerStatefulWidget {
  const BoardScreen({super.key});

  @override
  ConsumerState<BoardScreen> createState() => _BoardScreenState();
}

class _BoardScreenState extends ConsumerState<BoardScreen> {
  String? _reservingId;

  Future<void> _reserve(Load load, S s) async {
    setState(() => _reservingId = load.id);
    try {
      await ref.read(loadRepositoryProvider).reserve(load.id);
      if (!mounted) return;
      _toast('${load.referenceNo}: ${s.reserved}', ok: true);
      // The load leaves the published board on success, so the list must
      // re-fetch rather than patch the card in place.
      ref.invalidate(loadsProvider);
      ref.invalidate(bookingsProvider);
    } on ApiException catch (e) {
      if (!mounted) return;
      _toast(e.message, ok: false);
    } on SessionExpired {
      if (!mounted) return;
      _toast(s.sessionExpired, ok: false);
    } finally {
      if (mounted) setState(() => _reservingId = null);
    }
  }

  void _toast(String message, {required bool ok}) {
    final ColorScheme scheme = Theme.of(context).colorScheme;
    ScaffoldMessenger.of(context)
      ..hideCurrentSnackBar()
      ..showSnackBar(
        SnackBar(
          content: Text(message),
          behavior: SnackBarBehavior.floating,
          backgroundColor: ok ? scheme.secondary : scheme.error,
        ),
      );
  }

  @override
  Widget build(BuildContext context) {
    final S s = ref.watch(stringsProvider);
    final Fmt fmt = ref.watch(fmtProvider);
    final LoadFilters filters = ref.watch(loadFiltersProvider);
    final AsyncValue<Paged<Load>> loads = ref.watch(loadsProvider(filters));
    final CurrentUser? user = ref.watch(authProvider);
    final bool canReserve = user?.can('bookings.create') ?? false;

    return Scaffold(
      appBar: AppBar(
        title: Text(s.boardTitle),
        actions: <Widget>[
          IconButton(
            tooltip: s.filters,
            onPressed: () => _openFilters(context, s),
            icon: Badge(
              isLabelVisible: !filters.isEmpty,
              child: const Icon(Icons.tune_rounded),
            ),
          ),
        ],
      ),
      body: RefreshIndicator(
        onRefresh: () async => ref.invalidate(loadsProvider),
        child: loads.when(
          loading: () => const SkeletonList(),
          error: (Object e, _) => ListView(
            // A plain Center inside RefreshIndicator cannot be pulled, so the
            // error state stays scrollable and therefore retryable by gesture.
            physics: const AlwaysScrollableScrollPhysics(),
            children: <Widget>[
              SizedBox(
                height: MediaQuery.sizeOf(context).height * 0.6,
                child: ErrorView(
                  error: e,
                  s: s,
                  onRetry: () => ref.invalidate(loadsProvider),
                ),
              ),
            ],
          ),
          data: (Paged<Load> page) {
            if (page.items.isEmpty) {
              return ListView(
                physics: const AlwaysScrollableScrollPhysics(),
                children: <Widget>[
                  SizedBox(
                    height: MediaQuery.sizeOf(context).height * 0.6,
                    child: EmptyView(
                      title: s.emptyBoard,
                      hint: s.emptyBoardHint,
                      icon: Icons.local_shipping_outlined,
                    ),
                  ),
                ],
              );
            }
            return ListView.separated(
              padding: const EdgeInsets.fromLTRB(16, 12, 16, 24),
              physics: const AlwaysScrollableScrollPhysics(),
              itemCount: page.items.length + 1,
              separatorBuilder: (_, _) => const SizedBox(height: 12),
              itemBuilder: (BuildContext context, int i) {
                if (i == 0) {
                  return Padding(
                    padding: const EdgeInsets.only(bottom: 4),
                    child: Text(
                      '${fmt.number(page.meta.total)} ${s.loadsFound}',
                      style: Theme.of(context).textTheme.bodySmall,
                    ),
                  );
                }
                final Load load = page.items[i - 1];
                return LoadCard(
                  load: load,
                  s: s,
                  fmt: fmt,
                  reserving: _reservingId == load.id,
                  onTap: () => context.push('/loads/${load.id}'),
                  onReserve: canReserve ? () => _reserve(load, s) : null,
                );
              },
            );
          },
        ),
      ),
    );
  }

  void _openFilters(BuildContext context, S s) {
    unawaited(
      showModalBottomSheet<void>(
        context: context,
        isScrollControlled: true,
        showDragHandle: true,
        builder: (_) => _FilterSheet(s: s),
      ),
    );
  }
}

/// Filters live in a sheet rather than above the list: on a phone the board is
/// the scarce resource, and a driver changes filters far less often than they
/// scroll.
class _FilterSheet extends ConsumerWidget {
  const _FilterSheet({required this.s});

  final S s;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final LoadFilters filters = ref.watch(loadFiltersProvider);
    final AsyncValue<List<RefItem>> regions = ref.watch(regionsProvider);
    final AsyncValue<List<RefItem>> vehicles = ref.watch(vehicleTypesProvider);
    final CurrentUser? user = ref.watch(authProvider);

    void update(LoadFilters next) =>
        ref.read(loadFiltersProvider.notifier).state = next;

    return Padding(
      padding: EdgeInsets.only(
        left: 16,
        right: 16,
        bottom: MediaQuery.viewInsetsOf(context).bottom + 24,
      ),
      child: SingleChildScrollView(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          mainAxisSize: MainAxisSize.min,
          children: <Widget>[
            Text(s.filters, style: Theme.of(context).textTheme.titleLarge),
            const SizedBox(height: 16),
            _RegionField(
              label: s.from,
              placeholder: s.anyRegion,
              regions: regions,
              value: filters.pickupRegionId,
              onChanged: (String? id) =>
                  update(filters.copyWith(pickupRegionId: id)),
            ),
            const SizedBox(height: 12),
            _RegionField(
              label: s.to,
              placeholder: s.anyRegion,
              regions: regions,
              value: filters.deliveryRegionId,
              onChanged: (String? id) =>
                  update(filters.copyWith(deliveryRegionId: id)),
            ),
            const SizedBox(height: 12),
            _RegionField(
              label: s.vehicleType,
              placeholder: s.anyVehicle,
              regions: vehicles,
              value: filters.vehicleType,
              onChanged: (String? code) =>
                  update(filters.copyWith(vehicleType: code)),
            ),
            const SizedBox(height: 12),
            Row(
              children: <Widget>[
                Expanded(
                  child: _WeightField(
                    label: s.minWeight,
                    value: filters.minWeightKg,
                    onChanged: (int? kg) =>
                        update(filters.copyWith(minWeightKg: kg)),
                  ),
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: _WeightField(
                    label: s.maxWeight,
                    value: filters.maxWeightKg,
                    onChanged: (int? kg) =>
                        update(filters.copyWith(maxWeightKg: kg)),
                  ),
                ),
              ],
            ),
            // Only a driver has a vehicle to match against, so the switch is
            // absent rather than disabled for everyone else.
            if (user?.isDriver ?? false)
              SwitchListTile(
                contentPadding: EdgeInsets.zero,
                title: Text(s.fitsMyVehicle),
                value: filters.fitsMyVehicle,
                onChanged: (bool v) =>
                    update(filters.copyWith(fitsMyVehicle: v)),
              ),
            const SizedBox(height: 8),
            Row(
              children: <Widget>[
                Expanded(
                  child: OutlinedButton(
                    onPressed: () => update(const LoadFilters()),
                    child: Text(s.clearFilters),
                  ),
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: FilledButton(
                    onPressed: () => Navigator.of(context).pop(),
                    child: Text(s.board),
                  ),
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }
}

class _RegionField extends StatelessWidget {
  const _RegionField({
    required this.label,
    required this.placeholder,
    required this.regions,
    required this.value,
    required this.onChanged,
  });

  final String label;
  final String placeholder;
  final AsyncValue<List<RefItem>> regions;
  final String? value;
  final ValueChanged<String?> onChanged;

  @override
  Widget build(BuildContext context) {
    final List<RefItem> items = regions.valueOrNull ?? const <RefItem>[];
    // Guard against a stale selection: if the vocabulary reloaded in another
    // language the ids are the same, but if it failed to load the current
    // value would have no matching item and DropdownButton would assert.
    final bool valid = value != null && items.any((RefItem r) => r.id == value);

    return DropdownButtonFormField<String?>(
      initialValue: valid ? value : null,
      decoration: InputDecoration(labelText: label),
      isExpanded: true,
      items: <DropdownMenuItem<String?>>[
        DropdownMenuItem<String?>(child: Text(placeholder)),
        ...items.map(
          (RefItem r) => DropdownMenuItem<String?>(
            value: r.id,
            child: Text(r.name, overflow: TextOverflow.ellipsis),
          ),
        ),
      ],
      onChanged: onChanged,
    );
  }
}

class _WeightField extends StatelessWidget {
  const _WeightField({
    required this.label,
    required this.value,
    required this.onChanged,
  });

  final String label;
  final int? value;
  final ValueChanged<int?> onChanged;

  @override
  Widget build(BuildContext context) {
    return TextFormField(
      initialValue: value?.toString(),
      decoration: InputDecoration(labelText: label),
      keyboardType: TextInputType.number,
      onChanged: (String text) =>
          onChanged(text.isEmpty ? null : int.tryParse(text)),
    );
  }
}
