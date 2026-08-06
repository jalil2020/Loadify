import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../app/locale.dart';
import '../../../app/providers.dart';
import '../../../core/error/api_exception.dart';
import '../../../core/format/formatters.dart';
import '../../../core/widgets/states.dart';
import '../../../l10n/strings.dart';
import '../../auth/domain/current_user.dart';
import '../../trips/domain/trip.dart';
import '../domain/driver_profile.dart';

class ProfileScreen extends ConsumerStatefulWidget {
  const ProfileScreen({super.key});

  @override
  ConsumerState<ProfileScreen> createState() => _ProfileScreenState();
}

class _ProfileScreenState extends ConsumerState<ProfileScreen> {
  bool _togglingStatus = false;

  Future<void> _setStatus(bool online) async {
    setState(() => _togglingStatus = true);
    try {
      await ref.read(driverRepositoryProvider).setStatus(online: online);
      if (!mounted) return;
      ref.invalidate(driverProfileProvider);
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
      if (mounted) setState(() => _togglingStatus = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final S s = ref.watch(stringsProvider);
    final Fmt fmt = ref.watch(fmtProvider);
    final CurrentUser? user = ref.watch(authProvider);
    final AsyncValue<DriverProfile> profile = ref.watch(driverProfileProvider);
    final AsyncValue<Wallet> wallet = ref.watch(walletProvider);
    final ColorScheme scheme = Theme.of(context).colorScheme;

    return Scaffold(
      appBar: AppBar(title: Text(s.profile)),
      body: RefreshIndicator(
        onRefresh: () async {
          ref
            ..invalidate(driverProfileProvider)
            ..invalidate(walletProvider);
        },
        child: ListView(
          padding: const EdgeInsets.fromLTRB(16, 12, 16, 32),
          physics: const AlwaysScrollableScrollPhysics(),
          children: <Widget>[
            _Header(user: user, profile: profile.valueOrNull, s: s, fmt: fmt),
            const SizedBox(height: 16),

            // Only a driver has an availability status to set. A company user
            // signed in on the phone sees the rest of the screen without a
            // toggle that would mean nothing for them.
            if (user?.isDriver ?? false) ...<Widget>[
              Card(
                child: profile.when(
                  loading: () => const ListTile(
                    title: Text('...'),
                    trailing: SizedBox(
                      width: 20,
                      height: 20,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    ),
                  ),
                  error: (Object e, _) => ListTile(
                    title: Text(s.error),
                    subtitle: Text(e is ApiException ? e.message : ''),
                    trailing: TextButton(
                      onPressed: () => ref.invalidate(driverProfileProvider),
                      child: Text(s.retry),
                    ),
                  ),
                  data: (DriverProfile p) => SwitchListTile(
                    title: Text(s.goOnline),
                    subtitle: Text(switch (p.status) {
                      DriverStatus.online => s.online,
                      DriverStatus.busy => s.busy,
                      DriverStatus.offline => s.offline,
                    }),
                    // `busy` is server-assigned while a trip runs; letting the
                    // driver flip it here would fight the trip state machine.
                    value: p.status.isAvailable,
                    onChanged:
                        (_togglingStatus || p.status == DriverStatus.busy)
                        ? null
                        : _setStatus,
                  ),
                ),
              ),
              const SizedBox(height: 12),
            ],

            Card(
              child: wallet.when(
                loading: () => const ListTile(title: Text('...')),
                error: (Object e, _) => ListTile(
                  title: Text(s.wallet),
                  subtitle: Text(e is ApiException ? e.message : s.error),
                  trailing: TextButton(
                    onPressed: () => ref.invalidate(walletProvider),
                    child: Text(s.retry),
                  ),
                ),
                data: (Wallet w) => Padding(
                  padding: const EdgeInsets.all(14),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: <Widget>[
                      Text(
                        s.wallet,
                        style: Theme.of(context).textTheme.titleSmall,
                      ),
                      const SizedBox(height: 10),
                      Row(
                        children: <Widget>[
                          Expanded(
                            child: _Metric(
                              label: s.available,
                              value: fmt.money(w.available, w.currency),
                              emphasis: true,
                            ),
                          ),
                          Expanded(
                            child: _Metric(
                              label: s.held,
                              value: fmt.money(w.heldBalance, w.currency),
                            ),
                          ),
                        ],
                      ),
                    ],
                  ),
                ),
              ),
            ),
            const SizedBox(height: 12),

            if (profile.valueOrNull != null &&
                profile.value!.vehicles.isNotEmpty) ...<Widget>[
              Card(
                child: Padding(
                  padding: const EdgeInsets.all(14),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: <Widget>[
                      Text(
                        s.vehicles,
                        style: Theme.of(context).textTheme.titleSmall,
                      ),
                      const SizedBox(height: 8),
                      ...profile.value!.vehicles.map(
                        (Vehicle v) => Padding(
                          padding: const EdgeInsets.symmetric(vertical: 5),
                          child: Row(
                            children: <Widget>[
                              Icon(
                                Icons.local_shipping_outlined,
                                size: 18,
                                color: scheme.onSurfaceVariant,
                              ),
                              const SizedBox(width: 10),
                              Expanded(
                                child: Text(
                                  '${v.label} · ${v.plateNumber}',
                                  maxLines: 1,
                                  overflow: TextOverflow.ellipsis,
                                ),
                              ),
                              Text(
                                fmt.weight(v.capacityKg),
                                style: Theme.of(context).textTheme.labelSmall
                                    ?.copyWith(color: scheme.onSurfaceVariant),
                              ),
                            ],
                          ),
                        ),
                      ),
                    ],
                  ),
                ),
              ),
              const SizedBox(height: 12),
            ],

            Card(
              child: Column(
                children: <Widget>[
                  ListTile(
                    leading: const Icon(Icons.language_rounded),
                    title: Text(s.language),
                    trailing: DropdownButton<String>(
                      value: ref.watch(localeProvider),
                      underline: const SizedBox.shrink(),
                      items: S.supported
                          .map(
                            (String code) => DropdownMenuItem<String>(
                              value: code,
                              child: Text(S.languageNames[code] ?? code),
                            ),
                          )
                          .toList(),
                      onChanged: (String? code) {
                        if (code != null) {
                          unawaited(
                            ref.read(localeProvider.notifier).set(code),
                          );
                        }
                      },
                    ),
                  ),
                  const Divider(height: 1),
                  ListTile(
                    leading: Icon(Icons.logout_rounded, color: scheme.error),
                    title: Text(
                      s.signOut,
                      style: TextStyle(color: scheme.error),
                    ),
                    onTap: () =>
                        unawaited(ref.read(authProvider.notifier).signOut()),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _Header extends StatelessWidget {
  const _Header({
    required this.user,
    required this.profile,
    required this.s,
    required this.fmt,
  });

  final CurrentUser? user;
  final DriverProfile? profile;
  final S s;
  final Fmt fmt;

  @override
  Widget build(BuildContext context) {
    final ColorScheme scheme = Theme.of(context).colorScheme;
    final TextTheme text = Theme.of(context).textTheme;

    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          children: <Widget>[
            Row(
              children: <Widget>[
                CircleAvatar(
                  radius: 26,
                  backgroundColor: scheme.primary,
                  child: Text(
                    user?.initials ?? '?',
                    style: TextStyle(
                      color: scheme.onPrimary,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                ),
                const SizedBox(width: 14),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: <Widget>[
                      Text(
                        user?.fullName ?? '—',
                        style: text.titleMedium?.copyWith(
                          fontWeight: FontWeight.w600,
                        ),
                      ),
                      Text(
                        Fmt.phone(user?.phone),
                        style: text.bodySmall?.copyWith(
                          color: scheme.onSurfaceVariant,
                        ),
                      ),
                    ],
                  ),
                ),
                if (profile != null)
                  StatusBadge(
                    label: profile!.isVerified ? s.verified : s.unverified,
                    color: profile!.isVerified
                        ? scheme.secondary
                        : scheme.outline,
                  ),
              ],
            ),
            if (profile != null) ...<Widget>[
              const SizedBox(height: 16),
              Row(
                children: <Widget>[
                  Expanded(
                    child: _Metric(
                      label: s.rating,
                      value: '★ ${profile!.rating.toStringAsFixed(1)}',
                    ),
                  ),
                  Expanded(
                    child: _Metric(
                      label: s.completedTrips,
                      value: fmt.number(profile!.completedTrips),
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

class _Metric extends StatelessWidget {
  const _Metric({
    required this.label,
    required this.value,
    this.emphasis = false,
  });

  final String label;
  final String value;
  final bool emphasis;

  @override
  Widget build(BuildContext context) {
    final ColorScheme scheme = Theme.of(context).colorScheme;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: <Widget>[
        Text(
          label,
          style: Theme.of(
            context,
          ).textTheme.labelSmall?.copyWith(color: scheme.onSurfaceVariant),
        ),
        const SizedBox(height: 2),
        Text(
          value,
          style: Theme.of(context).textTheme.titleMedium?.copyWith(
            fontWeight: FontWeight.w700,
            color: emphasis ? scheme.secondary : null,
          ),
        ),
      ],
    );
  }
}
