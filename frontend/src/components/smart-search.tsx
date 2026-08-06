"use client";

import * as React from "react";

import { Button, Input } from "@/components/ui/primitives";
import { useLocale } from "@/components/providers";
import {
  useParseSearch,
  type LoadFilters,
  type ParsedChip,
} from "@/hooks/queries";
import { ApiError } from "@/lib/api";
import { formatWeight, formatMoney, formatDate } from "@/lib/format";
import type { Region, VehicleTypeOption } from "@/lib/types";

/**
 * Rebuilds the board's filters from the chips that survived.
 *
 * The chips are the source of truth for a parsed search, not a decoration over
 * one: removing a chip drops exactly the query parameter it names, so what is
 * on screen and what is being searched cannot disagree.
 */
export function filtersFromChips(chips: ParsedChip[]): LoadFilters {
  const filters: LoadFilters = {};

  for (const chip of chips) {
    switch (chip.field) {
      case "pickup_region_id":
        filters.pickup_region_id = chip.value;
        break;
      case "delivery_region_id":
        filters.delivery_region_id = chip.value;
        break;
      case "vehicle_type":
        filters.vehicle_type = chip.value;
        break;
      case "min_weight_kg":
        filters.min_weight_kg = Number(chip.value);
        break;
      case "max_weight_kg":
        filters.max_weight_kg = Number(chip.value);
        break;
      case "min_price":
        filters.min_price = Number(chip.value);
        break;
      case "max_price":
        filters.max_price = Number(chip.value);
        break;
      case "pickup_from":
        filters.pickup_from = chip.value;
        break;
      case "pickup_to":
        filters.pickup_to = chip.value;
        break;
      case "requires_adr":
        filters.requires_adr = true;
        break;
      case "requires_refrigeration":
        filters.requires_refrigeration = true;
        break;
      // A field the client does not know how to apply is deliberately not
      // guessed at. It stays off the board rather than being sent blind.
    }
  }
  return filters;
}

/**
 * Turns a chip's raw value into something readable.
 *
 * A region chip carries a UUID, which is meaningless on screen. The board has
 * already fetched the vocabularies for its dropdowns, so resolving here reuses
 * that rather than making the server send every name twice.
 */
function chipDisplay(
  chip: ParsedChip,
  regions: Region[],
  vehicleTypes: VehicleTypeOption[],
  locale: string,
): string {
  switch (chip.field) {
    case "pickup_region_id":
    case "delivery_region_id":
      return regions.find((r) => r.id === chip.value)?.name ?? chip.value;
    case "vehicle_type":
      return vehicleTypes.find((v) => v.code === chip.value)?.name ?? chip.value;
    case "min_weight_kg":
    case "max_weight_kg":
      return formatWeight(Number(chip.value), locale as never);
    case "min_price":
    case "max_price":
      return formatMoney(Number(chip.value), "UZS", locale as never);
    case "pickup_from":
    case "pickup_to":
      return formatDate(chip.value, locale as never);
    default:
      return chip.value;
  }
}

interface SmartSearchProps {
  regions: Region[];
  vehicleTypes: VehicleTypeOption[];
  /** Called with the filters the surviving chips describe. */
  onApply: (filters: LoadFilters) => void;
  /** Called when the search is cleared, so the board returns to unfiltered. */
  onClear: () => void;
  /**
   * Set when the board's own controls were touched. The chips no longer
   * describe what is being searched, so they are dropped rather than left
   * on screen saying something untrue.
   */
  invalidatedAt?: number;
}

/**
 * A single box that accepts a freight request the way someone would say it.
 *
 * The parse is shown before it is trusted: what came back appears as removable
 * chips, so a misread region is visible and correctable rather than silently
 * changing what was searched.
 */
export function SmartSearch({
  regions,
  vehicleTypes,
  onApply,
  onClear,
  invalidatedAt,
}: SmartSearchProps) {
  const { t, locale } = useLocale();
  const parse = useParseSearch();

  const [query, setQuery] = React.useState("");
  const [chips, setChips] = React.useState<ParsedChip[]>([]);
  const [dropped, setDropped] = React.useState<string[]>([]);
  const [error, setError] = React.useState<string | null>(null);

  // The board's own dropdowns moved, so the chips are stale.
  React.useEffect(() => {
    if (invalidatedAt) {
      setChips([]);
      setDropped([]);
    }
  }, [invalidatedAt]);

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    const trimmed = query.trim();
    if (trimmed.length < 2) return;

    setError(null);
    try {
      const result = await parse.mutateAsync({ query: trimmed, target: "loads" });

      if (result.empty) {
        // Nothing was understood, so the board keeps whatever it was showing —
        // and the chips describing it stay put. Clearing them while leaving the
        // filters applied would put a filtered board on screen with nothing to
        // explain it, which is worse than the failed search itself.
        setError(t("search.notUnderstood"));
        return;
      }

      const next = result.chips ?? [];
      setChips(next);
      setDropped(result.dropped ?? []);
      onApply(filtersFromChips(next));
    } catch (e) {
      setError(e instanceof ApiError ? e.message : t("common.error"));
    }
  };

  const removeChip = (index: number) => {
    const next = chips.filter((_, i) => i !== index);
    setChips(next);
    if (next.length === 0) {
      onClear();
      return;
    }
    onApply(filtersFromChips(next));
  };

  const clearAll = () => {
    setQuery("");
    setChips([]);
    setDropped([]);
    setError(null);
    onClear();
  };

  return (
    <div className="space-y-2">
      <form onSubmit={submit} className="flex gap-2">
        <Input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder={t("search.placeholder")}
          aria-label={t("search.label")}
          className="flex-1"
        />
        <Button type="submit" loading={parse.isPending} disabled={query.trim().length < 2}>
          {t("search.submit")}
        </Button>
        {(chips.length > 0 || query) && (
          <Button type="button" variant="outline" onClick={clearAll}>
            {t("filter.reset")}
          </Button>
        )}
      </form>

      {chips.length > 0 && (
        <div className="flex flex-wrap items-center gap-1.5">
          {chips.map((chip, index) => (
            <button
              key={`${chip.field}-${chip.value}-${index}`}
              type="button"
              onClick={() => removeChip(index)}
              className="group inline-flex items-center gap-1.5 rounded-md border border-primary/30 bg-primary/10 px-2.5 py-1 text-xs text-foreground transition hover:border-destructive/40 hover:bg-destructive/10"
              aria-label={`${chip.label}: ${chipDisplay(chip, regions, vehicleTypes, locale)} — ${t("search.removeChip")}`}
            >
              <span className="text-muted-foreground">{chip.label}</span>
              <span className="font-medium">
                {chipDisplay(chip, regions, vehicleTypes, locale)}
              </span>
              <span aria-hidden className="text-muted-foreground group-hover:text-destructive">
                ×
              </span>
            </button>
          ))}
        </div>
      )}

      {dropped.length > 0 && (
        // Saying what was ignored is the point: a search that quietly dropped
        // a region reads as a search that found nothing there.
        <p className="text-xs text-muted-foreground">
          {t("search.dropped")} {dropped.join(", ")}
        </p>
      )}

      {error && (
        <p role="alert" className="text-xs text-destructive">
          {error}
        </p>
      )}
    </div>
  );
}
