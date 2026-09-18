import type { LoadAd } from "../../lib/types";

/**
 * The first line of the advert, which is how people tell their adverts apart
 * in a list. Cut by characters rather than UTF-16 units, so an emoji at the
 * edge is kept or dropped whole.
 */
export function adTitle(ad: Pick<LoadAd, "raw_text" | "origin" | "destination">, max = 80): string {
  const line = (ad.raw_text ?? "")
    .split("\n")
    .map((l) => l.trim())
    .find(Boolean);
  if (line) {
    const chars = Array.from(line);
    return chars.length > max ? `${chars.slice(0, max - 1).join("").trimEnd()}…` : line;
  }
  // An advert created through the API from fields alone has no text of its own.
  const from = ad.origin.text || ad.origin.country || "?";
  const to = ad.destination.text || ad.destination.country || "?";
  return `${from} → ${to}`;
}
