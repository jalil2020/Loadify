// Every time is shown in the platform's timezone, whatever the browser's is:
// "loading tomorrow" means tomorrow in Tashkent.
export const TIMEZONE = "Asia/Tashkent";

const numberFormat = new Intl.NumberFormat("ru-RU");

export function formatNumber(n: number | null | undefined): string {
  if (n === null || n === undefined) return "—";
  return numberFormat.format(n);
}

function parts(iso: string) {
  const d = new Date(iso);
  const get = (opts: Intl.DateTimeFormatOptions) =>
    new Intl.DateTimeFormat("en-GB", { timeZone: TIMEZONE, ...opts }).format(d);
  return {
    day: get({ day: "2-digit" }),
    month: get({ month: "2-digit" }),
    year: get({ year: "numeric" }),
    time: get({ hour: "2-digit", minute: "2-digit", hour12: false }),
  };
}

/** 18.09.2026 */
export function formatDate(iso: string | null | undefined): string {
  if (!iso) return "—";
  // A plain calendar day is not a moment; it is shown as written.
  const plain = /^(\d{4})-(\d{2})-(\d{2})$/.exec(iso);
  if (plain) return `${plain[3]}.${plain[2]}.${plain[1]}`;
  const p = parts(iso);
  return `${p.day}.${p.month}.${p.year}`;
}

/** 18.09.2026 14:30 */
export function formatDateTime(iso: string | null | undefined): string {
  if (!iso) return "—";
  const p = parts(iso);
  return `${p.day}.${p.month}.${p.year} ${p.time}`;
}

/** 14:30 today, or the date and time otherwise. */
export function formatWhen(iso: string | null | undefined, now = new Date()): string {
  if (!iso) return "—";
  const p = parts(iso);
  const today = parts(now.toISOString());
  if (p.day === today.day && p.month === today.month && p.year === today.year) return p.time;
  return `${p.day}.${p.month} ${p.time}`;
}

/** 1 h 5 min, 3 min, 40 s — for "retry after". */
export function formatDuration(seconds: number, lang: "uz" | "ru"): string {
  const units =
    lang === "ru" ? { h: "ч", m: "мин", s: "с" } : { h: "soat", m: "daq.", s: "son." };
  if (seconds < 60) return `${Math.max(1, Math.round(seconds))} ${units.s}`;
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return `${minutes} ${units.m}`;
  const h = Math.floor(minutes / 60);
  const m = minutes % 60;
  return m ? `${h} ${units.h} ${m} ${units.m}` : `${h} ${units.h}`;
}

/** Amounts travel in minor units; 250000 USD is 2 500 USD. */
export function formatMoney(minor: number | null | undefined, currency: string | null | undefined): string {
  if (minor === null || minor === undefined) return "—";
  const major = minor / 100;
  const text = Number.isInteger(major)
    ? numberFormat.format(major)
    : new Intl.NumberFormat("ru-RU", { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(major);
  return currency ? `${text} ${currency}` : text;
}

export function toMinor(major: number): number {
  return Math.round(major * 100);
}

export function fromMinor(minor: number | null | undefined): number | null {
  return minor === null || minor === undefined ? null : minor / 100;
}

/** 20 000 kg reads better as 20 t. */
export function formatWeight(kg: number | null | undefined): string {
  if (kg === null || kg === undefined) return "—";
  if (kg >= 1000) {
    const t = kg / 1000;
    return `${Number.isInteger(t) ? t : t.toFixed(1)} t`;
  }
  return `${numberFormat.format(kg)} kg`;
}

export function formatPercent(ratio: number | null | undefined): string {
  if (ratio === null || ratio === undefined) return "—";
  return `${Math.round(ratio * 100)}%`;
}

export function formatUSD(amount: number): string {
  return `$${amount.toFixed(amount < 10 ? 4 : 2)}`;
}
