import type { Locale } from "@/lib/types";

const INTL_LOCALE: Record<Locale, string> = {
  uz: "uz-UZ",
  ru: "ru-RU",
  en: "en-GB",
};

/**
 * Uzbek is the locale most likely to be missing from a trimmed ICU build, and
 * this market runs on low-end Android where trimmed builds are common. When the
 * data is absent Intl does not throw — it silently falls back to the root
 * locale, rendering "2026 M08 5" and "+24 h" where a user expects "5-avg, 2026"
 * and "24 soatdan keyin".
 *
 * supportedLocalesOf cannot detect this: browsers observed in testing report
 * "uz-UZ" as supported and still produce root output. So the locale is probed
 * by formatting known values and looking for the root locale's signatures —
 * a numeric "M08" month, and a "+24 h"-style signed duration.
 */
const FALLBACK_CHAIN: Record<Locale, string[]> = {
  uz: ["uz-UZ", "ru-RU", "en-GB"],
  ru: ["ru-RU", "en-GB"],
  en: ["en-GB"],
};

/** Root-locale month, e.g. "2026 M08 5". A real locale spells the month out. */
const ROOT_MONTH = /M\d{2}/;
/** Root-locale duration, e.g. "+24 h" or "-58 min". */
const ROOT_DURATION = /^[+-]/;

const PROBE_DATE = new Date(Date.UTC(2026, 7, 5));

function rendersProperly(tag: string): boolean {
  try {
    const date = new Intl.DateTimeFormat(tag, {
      day: "numeric",
      month: "short",
      year: "numeric",
    }).format(PROBE_DATE);
    if (ROOT_MONTH.test(date)) return false;

    const duration = new Intl.RelativeTimeFormat(tag, { numeric: "auto" }).format(24, "hour");
    return !ROOT_DURATION.test(duration);
  } catch {
    return false;
  }
}

// Probing builds several formatters, so it runs once per locale rather than per
// cell — these functions are called for every field of every row.
const resolved = new Map<Locale, string>();

function intlLocale(locale: Locale): string {
  const cached = resolved.get(locale);
  if (cached) return cached;

  const pick = FALLBACK_CHAIN[locale].find(rendersProperly) ?? INTL_LOCALE[locale];
  resolved.set(locale, pick);
  return pick;
}

/**
 * Renders minor units as money.
 *
 * The API stores tiyin, so 350_000_00 is 350 000 soʻm. UZS is shown without
 * decimals because tiyin are not used in practice and two trailing zeros on
 * every price is noise.
 */
export function formatMoney(minor: number, currency = "UZS", locale: Locale = "uz"): string {
  const major = minor / 100;
  const fractionDigits = currency === "UZS" ? 0 : 2;
  return new Intl.NumberFormat(intlLocale(locale), {
    style: "currency",
    currency,
    minimumFractionDigits: fractionDigits,
    maximumFractionDigits: fractionDigits,
  }).format(major);
}

/** Compact form for tiles: 12,4 mln so'm. */
export function formatMoneyCompact(minor: number, currency = "UZS", locale: Locale = "uz"): string {
  const major = minor / 100;
  return new Intl.NumberFormat(intlLocale(locale), {
    style: "currency",
    currency,
    notation: "compact",
    maximumFractionDigits: 1,
  }).format(major);
}

export function formatNumber(value: number, locale: Locale = "uz"): string {
  return new Intl.NumberFormat(intlLocale(locale)).format(value);
}

/** Weight in kilograms, switching to tonnes once that reads better. */
export function formatWeight(kg: number, locale: Locale = "uz"): string {
  if (kg >= 1000) {
    const tonnes = kg / 1000;
    return `${new Intl.NumberFormat(intlLocale(locale), { maximumFractionDigits: 1 }).format(tonnes)} t`;
  }
  return `${formatNumber(kg, locale)} kg`;
}

export function formatDistance(km: number | null | undefined, locale: Locale = "uz"): string {
  if (km === null || km === undefined) return "—";
  return `${new Intl.NumberFormat(intlLocale(locale), { maximumFractionDigits: 0 }).format(km)} km`;
}

export function formatDate(iso: string | null | undefined, locale: Locale = "uz"): string {
  if (!iso) return "—";
  return new Intl.DateTimeFormat(intlLocale(locale), {
    day: "numeric",
    month: "short",
    year: "numeric",
  }).format(new Date(iso));
}

export function formatDateTime(iso: string | null | undefined, locale: Locale = "uz"): string {
  if (!iso) return "—";
  return new Intl.DateTimeFormat(intlLocale(locale), {
    day: "numeric",
    month: "short",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(iso));
}

/**
 * Relative time, which is what a load board actually needs: "3 soat oldin"
 * tells a driver more than a timestamp does.
 */
export function formatRelative(iso: string | null | undefined, locale: Locale = "uz"): string {
  if (!iso) return "—";
  const diffMs = new Date(iso).getTime() - Date.now();
  const rtf = new Intl.RelativeTimeFormat(intlLocale(locale), { numeric: "auto" });

  const units: [Intl.RelativeTimeFormatUnit, number][] = [
    ["year", 365 * 24 * 3600_000],
    ["month", 30 * 24 * 3600_000],
    ["day", 24 * 3600_000],
    ["hour", 3600_000],
    ["minute", 60_000],
  ];
  for (const [unit, ms] of units) {
    if (Math.abs(diffMs) >= ms) {
      return rtf.format(Math.round(diffMs / ms), unit);
    }
  }
  return rtf.format(Math.round(diffMs / 1000), "second");
}

/** +998901234567 → +998 90 123 45 67 */
export function formatPhone(phone: string | null | undefined): string {
  if (!phone) return "—";
  const m = /^\+998(\d{2})(\d{3})(\d{2})(\d{2})$/.exec(phone);
  return m ? `+998 ${m[1]} ${m[2]} ${m[3]} ${m[4]}` : phone;
}

export function initials(fullName: string): string {
  return fullName
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0]?.toUpperCase() ?? "")
    .join("");
}
