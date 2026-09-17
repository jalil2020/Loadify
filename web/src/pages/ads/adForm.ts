import type { PlaceValue } from "../../components/places";
import { fromMinor, toMinor } from "../../lib/format";
import type { LoadAd, PlaceRef, SaveLoadAd } from "../../lib/types";

/** What the editor holds: strings for inputs, converted once on save. */
export interface AdFormValues {
  raw_text: string;
  origin: PlaceValue;
  destination: PlaceValue;
  cargo_type_id: string;
  cargo_text: string;
  weight_t: string;
  volume_m3: string;
  vehicle_type: string;
  loading_date: string;
  loading_date_text: string;
  price: string;
  price_currency: string;
  price_negotiable: boolean;
  payment_terms: string;
  payment_method: string;
  contact_phone: string;
  contact_name: string;
  notes: string;
}

export const emptyAd: AdFormValues = {
  raw_text: "",
  origin: { country: "", region_id: "", city_id: "", text: "" },
  destination: { country: "", region_id: "", city_id: "", text: "" },
  cargo_type_id: "",
  cargo_text: "",
  weight_t: "",
  volume_m3: "",
  vehicle_type: "",
  loading_date: "",
  loading_date_text: "",
  price: "",
  price_currency: "USD",
  price_negotiable: false,
  payment_terms: "",
  payment_method: "",
  contact_phone: "",
  contact_name: "",
  notes: "",
};

const str = (v: unknown): string => (v === null || v === undefined ? "" : String(v));

function place(ref: PlaceRef | undefined): PlaceValue {
  return {
    country: str(ref?.country),
    region_id: str(ref?.region_id),
    city_id: str(ref?.city_id),
    text: str(ref?.text),
  };
}

function tons(kg: number | null | undefined): string {
  return kg ? String(kg / 1000) : "";
}

export function fromAd(ad: LoadAd): AdFormValues {
  return {
    raw_text: str(ad.raw_text),
    origin: place(ad.origin),
    destination: place(ad.destination),
    cargo_type_id: str(ad.cargo_type_id),
    cargo_text: str(ad.cargo_text),
    weight_t: tons(ad.weight_kg),
    volume_m3: str(ad.volume_m3),
    vehicle_type: str(ad.vehicle_type),
    loading_date: str(ad.loading_date),
    loading_date_text: str(ad.loading_date_text),
    price: str(fromMinor(ad.price_amount)),
    price_currency: str(ad.price_currency) || "USD",
    price_negotiable: ad.price_negotiable,
    payment_terms: str(ad.payment_terms),
    payment_method: str(ad.payment_method),
    contact_phone: str(ad.contact_phone),
    contact_name: str(ad.contact_name),
    notes: str(ad.notes),
  };
}

/** A parse draft uses the request shape, with the place fields flattened. */
export function fromDraft(draft: SaveLoadAd): AdFormValues {
  return {
    raw_text: str(draft.raw_text),
    origin: {
      country: str(draft.origin_country),
      region_id: str(draft.origin_region_id),
      city_id: str(draft.origin_city_id),
      text: str(draft.origin_text),
    },
    destination: {
      country: str(draft.destination_country),
      region_id: str(draft.destination_region_id),
      city_id: str(draft.destination_city_id),
      text: str(draft.destination_text),
    },
    cargo_type_id: str(draft.cargo_type_id),
    cargo_text: str(draft.cargo_text),
    weight_t: tons(draft.weight_kg),
    volume_m3: str(draft.volume_m3),
    vehicle_type: str(draft.vehicle_type),
    loading_date: str(draft.loading_date),
    loading_date_text: str(draft.loading_date_text),
    price: str(fromMinor(draft.price_amount)),
    price_currency: str(draft.price_currency) || "USD",
    price_negotiable: Boolean(draft.price_negotiable),
    payment_terms: str(draft.payment_terms),
    payment_method: str(draft.payment_method),
    contact_phone: str(draft.contact_phone),
    contact_name: str(draft.contact_name),
    notes: str(draft.notes),
  };
}

const orNull = (v: string): string | null => (v.trim() === "" ? null : v.trim());

function num(v: string): number | null {
  const n = Number(v.replace(",", ".").replace(/\s/g, ""));
  return v.trim() === "" || Number.isNaN(n) ? null : n;
}

export function toRequest(v: AdFormValues): SaveLoadAd {
  const weight = num(v.weight_t);
  const price = num(v.price);
  return {
    raw_text: v.raw_text,
    origin_country: orNull(v.origin.country),
    origin_region_id: orNull(v.origin.region_id),
    origin_city_id: orNull(v.origin.city_id),
    origin_text: v.origin.text.trim(),
    destination_country: orNull(v.destination.country),
    destination_region_id: orNull(v.destination.region_id),
    destination_city_id: orNull(v.destination.city_id),
    destination_text: v.destination.text.trim(),
    cargo_type_id: orNull(v.cargo_type_id),
    cargo_text: orNull(v.cargo_text),
    weight_kg: weight === null ? null : Math.round(weight * 1000),
    volume_m3: num(v.volume_m3),
    vehicle_type: orNull(v.vehicle_type),
    loading_date: orNull(v.loading_date),
    loading_date_text: orNull(v.loading_date_text),
    price_amount: price === null ? null : toMinor(price),
    price_currency: price === null ? null : orNull(v.price_currency),
    price_negotiable: v.price_negotiable,
    payment_terms: orNull(v.payment_terms),
    payment_method: orNull(v.payment_method),
    contact_phone: orNull(v.contact_phone.replace(/[\s()-]/g, "")),
    contact_name: orNull(v.contact_name),
    notes: orNull(v.notes),
  };
}

/** "Toshkent → Moskva", from the words people wrote. */
export function routeLabel(ad: Pick<LoadAd, "origin" | "destination">): string {
  const from = ad.origin.text || ad.origin.country || "?";
  const to = ad.destination.text || ad.destination.country || "?";
  return `${from} → ${to}`;
}

export const CURRENCIES = ["USD", "UZS", "RUB", "KZT", "EUR"];
export const PAYMENT_TERMS = ["on_loading", "on_delivery", "prepayment", "deferred"] as const;
export const PAYMENT_METHODS = ["cash", "transfer", "card", "escrow"] as const;
