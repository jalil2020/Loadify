"use client";

import * as React from "react";
import { Badge, Button, Card } from "@/components/ui/primitives";
import { useLocale } from "@/components/providers";
import {
  formatDate,
  formatDistance,
  formatMoney,
  formatPhone,
  formatRelative,
  formatWeight,
} from "@/lib/format";
import type { Load, LoadStatus } from "@/lib/types";

const STATUS_TONE: Record<LoadStatus, "neutral" | "primary" | "success" | "warning" | "danger" | "accent"> = {
  draft: "neutral",
  published: "accent",
  reserved: "warning",
  assigned: "primary",
  in_transit: "primary",
  delivered: "success",
  completed: "success",
  cancelled: "danger",
  expired: "neutral",
};

export function LoadCard({
  load,
  onReserve,
  reserving,
  canReserve,
}: {
  load: Load;
  onReserve?: (load: Load) => void;
  reserving?: boolean;
  canReserve?: boolean;
}) {
  const { t, locale } = useLocale();

  const pickup = load.pickup_address?.region_name ?? load.pickup_address?.city_name ?? load.pickup_address?.line1 ?? "—";
  const delivery = load.delivery_address?.region_name ?? load.delivery_address?.city_name ?? load.delivery_address?.line1 ?? "—";

  // A live booking is what earns the shipper's phone number, so its presence is
  // the signal that this driver is already in the conversation.
  const contact = load.company?.phone;
  const alreadyBooked = Boolean(load.my_booking);

  return (
    <Card className="animate-fade-in transition-shadow hover:shadow-md">
      <div className="flex flex-col gap-4 p-5 lg:flex-row lg:items-center">
        {/* Route */}
        <div className="min-w-0 flex-1">
          <div className="mb-2 flex flex-wrap items-center gap-2">
            <span className="font-mono text-xs text-muted-foreground">{load.reference_no}</span>
            <Badge tone={STATUS_TONE[load.status]}>{load.status.replace(/_/g, " ")}</Badge>
            {load.is_urgent && <Badge tone="danger">{t("load.urgent")}</Badge>}
            {load.requires_adr && <Badge tone="warning">{t("load.adr")}</Badge>}
            {load.requires_refrigeration && <Badge tone="primary">{t("load.refrigerated")}</Badge>}
          </div>

          <h3 className="truncate font-semibold">{load.title}</h3>

          <div className="mt-3 flex items-center gap-3 text-sm">
            <div className="min-w-0">
              <p className="truncate font-medium">{pickup}</p>
              <p className="text-xs text-muted-foreground">{formatDate(load.pickup_from, locale)}</p>
            </div>
            <svg viewBox="0 0 24 24" className="h-4 w-8 shrink-0 text-accent" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden>
              <path d="M3 12h16m0 0-5-5m5 5-5 5" strokeLinecap="round" strokeLinejoin="round" />
            </svg>
            <div className="min-w-0">
              <p className="truncate font-medium">{delivery}</p>
              <p className="text-xs text-muted-foreground">{formatDistance(load.distance_km, locale)}</p>
            </div>
          </div>
        </div>

        {/* Specs */}
        <dl className="grid shrink-0 grid-cols-3 gap-x-6 gap-y-1 text-sm lg:grid-cols-2 lg:gap-x-8">
          <div>
            <dt className="text-xs text-muted-foreground">{t("load.weight")}</dt>
            <dd className="tnum font-medium">{formatWeight(load.weight_kg, locale)}</dd>
          </div>
          <div>
            <dt className="text-xs text-muted-foreground">{t("load.vehicle")}</dt>
            <dd className="font-medium capitalize">
              {load.required_vehicle_type?.replace(/_/g, " ") ?? "—"}
            </dd>
          </div>
          {load.pickup_distance_km != null && (
            <div>
              <dt className="text-xs text-muted-foreground">{t("load.distance")}</dt>
              <dd className="tnum font-medium">{formatDistance(load.pickup_distance_km, locale)}</dd>
            </div>
          )}
        </dl>

        {/* Price and action */}
        <div className="shrink-0 lg:w-52 lg:text-right">
          <p className="tnum text-lg font-semibold">
            {formatMoney(load.price_amount, load.price_currency, locale)}
          </p>
          {load.price_per_km != null && (
            <p className="tnum text-xs text-muted-foreground">
              {formatMoney(Math.round(load.price_per_km), load.price_currency, locale)} / {t("load.perKm")}
            </p>
          )}

          {load.company && (
            <p className="mt-2 truncate text-xs text-muted-foreground">
              {load.company.brand_name}
              {load.company.is_verified && <span className="ml-1 text-accent">✓</span>}
            </p>
          )}

          {/* The masked-contact state is shown explicitly rather than hidden:
              the driver should know the number exists and how to earn it. */}
          <p className="mt-1 text-xs">
            {contact ? (
              <a href={`tel:${contact}`} className="font-medium text-primary hover:underline">
                {formatPhone(contact)}
              </a>
            ) : (
              <span className="text-muted-foreground">{t("board.contactHidden")}</span>
            )}
          </p>

          {onReserve && (
            <Button
              variant={alreadyBooked ? "outline" : "accent"}
              size="sm"
              className="mt-3 w-full lg:w-auto"
              disabled={alreadyBooked || !canReserve}
              loading={reserving}
              onClick={() => onReserve(load)}
            >
              {alreadyBooked
                ? t("board.reserved")
                : reserving
                  ? t("board.reserving")
                  : t("board.reserve")}
            </Button>
          )}

          <p className="mt-2 text-[11px] text-muted-foreground">
            {formatRelative(load.published_at ?? load.created_at, locale)} · {load.views_count} {t("load.views")}
          </p>
        </div>
      </div>
    </Card>
  );
}

export function LoadCardSkeleton() {
  return (
    <Card className="p-5">
      <div className="flex flex-col gap-4 lg:flex-row lg:items-center">
        <div className="flex-1 space-y-2">
          <div className="skeleton h-3 w-28" />
          <div className="skeleton h-5 w-2/3" />
          <div className="skeleton h-4 w-1/2" />
        </div>
        <div className="space-y-2 lg:w-40">
          <div className="skeleton h-4 w-24" />
          <div className="skeleton h-4 w-20" />
        </div>
        <div className="space-y-2 lg:w-52">
          <div className="skeleton h-6 w-32" />
          <div className="skeleton h-8 w-full" />
        </div>
      </div>
    </Card>
  );
}
