import 'package:flutter/material.dart';

import '../../../app/theme.dart';
import '../../../core/format/formatters.dart';
import '../../../core/widgets/states.dart';
import '../../../l10n/strings.dart';
import '../domain/load.dart';

/// One row of the board.
///
/// The corridor (region to region) and the price lead, because that is what a
/// driver scans for; everything else is secondary. Masked contact details are
/// shown as an explicit "visible after booking" rather than hidden, so the
/// driver understands there is a number to be had.
class LoadCard extends StatelessWidget {
  const LoadCard({
    required this.load,
    required this.s,
    required this.fmt,
    required this.onTap,
    this.onReserve,
    this.reserving = false,
    super.key,
  });

  final Load load;
  final S s;
  final Fmt fmt;
  final VoidCallback onTap;
  final VoidCallback? onReserve;
  final bool reserving;

  @override
  Widget build(BuildContext context) {
    final ColorScheme scheme = Theme.of(context).colorScheme;
    final TextTheme text = Theme.of(context).textTheme;

    return Card(
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(14),
        child: Padding(
          padding: const EdgeInsets.all(14),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: <Widget>[
              Row(
                children: <Widget>[
                  Text(
                    load.referenceNo,
                    style: text.labelMedium?.copyWith(
                      color: scheme.onSurfaceVariant,
                      fontFeatures: const <FontFeature>[
                        FontFeature.tabularFigures(),
                      ],
                    ),
                  ),
                  const SizedBox(width: 8),
                  if (load.isUrgent)
                    StatusBadge(label: '!', color: scheme.error),
                  const Spacer(),
                  StatusBadge(
                    label: load.status,
                    color: LoadifyTheme.statusColor(load.status, scheme),
                  ),
                ],
              ),
              const SizedBox(height: 8),
              Text(
                load.title,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: text.titleMedium?.copyWith(fontWeight: FontWeight.w600),
              ),
              const SizedBox(height: 10),
              _Corridor(load: load, fmt: fmt),
              const SizedBox(height: 10),
              Row(
                crossAxisAlignment: CrossAxisAlignment.end,
                children: <Widget>[
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: <Widget>[
                        Text(
                          fmt.money(load.priceAmount, load.priceCurrency),
                          style: text.titleLarge?.copyWith(
                            fontWeight: FontWeight.w700,
                            color: scheme.secondary,
                          ),
                        ),
                        if (load.pricePerKm != null)
                          Text(
                            '${fmt.money(load.pricePerKm!, load.priceCurrency)} ${s.perKm}',
                            style: text.labelSmall?.copyWith(
                              color: scheme.onSurfaceVariant,
                            ),
                          ),
                      ],
                    ),
                  ),
                  if (onReserve != null)
                    FilledButton(
                      onPressed: (load.isBooked || reserving)
                          ? null
                          : onReserve,
                      child: reserving
                          ? const SizedBox(
                              width: 18,
                              height: 18,
                              child: CircularProgressIndicator(strokeWidth: 2),
                            )
                          : Text(load.isBooked ? s.reserved : s.reserve),
                    ),
                ],
              ),
              const SizedBox(height: 8),
              Row(
                children: <Widget>[
                  Expanded(
                    child: Text(
                      load.company?.brandName ?? '—',
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: text.bodySmall,
                    ),
                  ),
                  if (load.company?.isVerified ?? false)
                    Icon(
                      Icons.verified_rounded,
                      size: 15,
                      color: scheme.secondary,
                    ),
                  const SizedBox(width: 8),
                  Text(
                    load.company?.phone == null
                        ? s.contactHidden
                        : Fmt.phone(load.company?.phone),
                    style: text.labelSmall?.copyWith(
                      color: scheme.onSurfaceVariant,
                    ),
                  ),
                ],
              ),
            ],
          ),
        ),
      ),
    );
  }
}

/// Pickup to delivery, with weight and distance between them.
class _Corridor extends StatelessWidget {
  const _Corridor({required this.load, required this.fmt});

  final Load load;
  final Fmt fmt;

  @override
  Widget build(BuildContext context) {
    final ColorScheme scheme = Theme.of(context).colorScheme;
    final TextTheme text = Theme.of(context).textTheme;

    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: <Widget>[
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: <Widget>[
              Text(
                load.pickupAddress?.shortLabel ?? '—',
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: text.bodyMedium?.copyWith(fontWeight: FontWeight.w600),
              ),
              Text(
                fmt.date(load.pickupFrom),
                style: text.labelSmall?.copyWith(
                  color: scheme.onSurfaceVariant,
                ),
              ),
            ],
          ),
        ),
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 8),
          child: Column(
            children: <Widget>[
              Icon(
                Icons.arrow_forward_rounded,
                size: 16,
                color: scheme.outline,
              ),
              Text(
                fmt.distance(load.distanceKm),
                style: text.labelSmall?.copyWith(color: scheme.outline),
              ),
            ],
          ),
        ),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.end,
            children: <Widget>[
              Text(
                load.deliveryAddress?.shortLabel ?? '—',
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                textAlign: TextAlign.end,
                style: text.bodyMedium?.copyWith(fontWeight: FontWeight.w600),
              ),
              Text(
                fmt.weight(load.weightKg),
                style: text.labelSmall?.copyWith(
                  color: scheme.onSurfaceVariant,
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }
}
