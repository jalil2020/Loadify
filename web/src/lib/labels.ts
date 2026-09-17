import type { MessageKey, Translate } from "../i18n";
import { ApiError } from "./api";

// Codes the API returns, turned into words. Anything unknown is shown as it
// came, so a new server code is visible rather than silently blank.

const REASONS = new Set([
  "duplicate",
  "cooldown",
  "group_limit",
  "user_limit",
  "account_limit",
  "plan_limit",
  "quiet_hours",
  "stale",
  "group_unavailable",
  "already_queued",
  "flood_wait",
  "slowmode",
  "peer_flood",
  "write_forbidden",
]);

/** Why a post was held back. */
export function reasonText(code: string | undefined, t: Translate): string {
  if (!code) return "";
  if (REASONS.has(code)) return t(`reason.${code}` as MessageKey);
  if (/^FLOOD_WAIT/.test(code)) return t("reason.flood_wait");
  if (/^SLOWMODE/.test(code)) return t("reason.slowmode");
  if (code === "PEER_FLOOD") return t("reason.peer_flood");
  return code;
}

const MATCHES = new Set([
  "route_region",
  "route_country",
  "route_one_end",
  "vehicle_listed",
  "cargo_listed",
  "weight_in_band",
  "large_group",
  "posted_recently",
]);

/** Why a group ranked where it did. */
export function matchText(code: string, t: Translate): string {
  return MATCHES.has(code) ? t(`match.${code}` as MessageKey) : code;
}

const FIELDS = new Set([
  "origin",
  "destination",
  "cargo_type",
  "weight_kg",
  "volume_m3",
  "vehicle_type",
  "loading_date",
  "price",
  "payment_terms",
  "contact_phone",
]);

export function fieldText(code: string, t: Translate): string {
  return FIELDS.has(code) ? t(`field.${code}` as MessageKey) : code;
}

// ── errors ──────────────────────────────────────────────────────────

const KNOWN_CODES = new Set([
  "INVALID_CREDENTIALS",
  "PHONE_ALREADY_REGISTERED",
  "PHONE_NOT_VERIFIED",
  "ACCOUNT_LOCKED",
  "ACCOUNT_BLOCKED",
  "OTP_INVALID",
  "OTP_EXPIRED",
  "OTP_TOO_MANY_ATTEMPTS",
  "OTP_COOLDOWN",
  "RATE_LIMITED",
  "FORBIDDEN",
  "NOT_FOUND",
  "QUOTA_EXCEEDED",
  "SERVICE_UNAVAILABLE",
  "VALIDATION_ERROR",
]);

// The server writes these messages for the situation at hand ("your plan has
// 3 of 10 posts left"), so they are shown under the translated title.
const SPECIFIC_CODES = new Set(["QUOTA_EXCEEDED", "CONFLICT", "BAD_REQUEST", "FORBIDDEN", "SERVICE_UNAVAILABLE"]);

export function describeError(err: unknown, t: Translate): string {
  if (!(err instanceof ApiError)) return t("err.generic");
  if (err.code === "NETWORK") return t("err.network");
  if (err.code === "VALIDATION_ERROR" && err.details.length > 0) {
    return `${t("err.VALIDATION_ERROR")}: ${err.details.map((d) => `${d.field} — ${d.message}`).join("; ")}`;
  }
  const known = KNOWN_CODES.has(err.code) ? t(`err.${err.code}` as MessageKey) : null;
  if (known && SPECIFIC_CODES.has(err.code) && err.message) return `${known}. ${err.message}`;
  if (known) return known;
  return err.message || t("err.generic");
}

/** Field errors from a 422, keyed by the request field name. */
export function fieldErrors(err: unknown): Record<string, string> {
  if (!(err instanceof ApiError)) return {};
  return Object.fromEntries(err.details.map((d) => [d.field, d.message]));
}

// ── phone numbers ───────────────────────────────────────────────────

/** Keeps what people type readable while sending +998XXXXXXXXX. */
export function normalizePhone(value: string): string {
  const digits = value.replace(/[^\d+]/g, "");
  if (digits.startsWith("+")) return digits;
  if (digits.startsWith("998")) return `+${digits}`;
  if (digits.length === 9) return `+998${digits}`;
  return digits;
}

export const UZ_PHONE = /^\+998\d{9}$/;
export const E164 = /^\+[1-9]\d{7,14}$/;

// Must match TelegramConsentVersion on the server: the consent text in the
// dictionaries is what the person agrees to, and the server records which
// version that was.
export const TELEGRAM_CONSENT_VERSION = "2026-09";
