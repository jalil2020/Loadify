import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { interpolate } from "../i18n/core";
import { ru } from "../i18n/ru";
import { uz } from "../i18n/uz";
import { fromDraft, toRequest } from "../pages/ads/adForm";
import { ApiError, api, refreshSession, tokens } from "./api";
import { formatDate, formatDuration, formatMoney, formatWeight, formatWhen } from "./format";
import { describeError, normalizePhone, reasonText, UZ_PHONE } from "./labels";

const t = (key: keyof typeof uz, vars?: Record<string, string | number>) => interpolate(uz[key], vars);

describe("dictionaries", () => {
  it("have the same keys in every language", () => {
    expect(Object.keys(ru).sort()).toEqual(Object.keys(uz).sort());
  });

  it("use the same placeholders in every language", () => {
    const names = (s: string) => [...s.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort();
    for (const key of Object.keys(uz) as (keyof typeof uz)[]) {
      expect(names(ru[key]), key).toEqual(names(uz[key]));
    }
  });
});

describe("format", () => {
  it("shows times in Tashkent whatever the browser's zone", () => {
    // 20:30 UTC is already the next day in Tashkent.
    expect(formatDate("2026-09-17T20:30:00Z")).toBe("18.09.2026");
    expect(formatWhen("2026-09-17T20:30:00Z", new Date("2026-09-17T20:40:00Z"))).toBe("01:30");
  });

  it("shows a calendar day as written", () => {
    expect(formatDate("2026-09-18")).toBe("18.09.2026");
  });

  it("reads minor units", () => {
    expect(formatMoney(250000, "USD")).toBe("2 500 USD");
    expect(formatMoney(12345, "UZS")).toBe("123,45 UZS");
    expect(formatMoney(null, "USD")).toBe("—");
  });

  it("writes tonnes when it reads better", () => {
    expect(formatWeight(20000)).toBe("20 t");
    expect(formatWeight(1500)).toBe("1.5 t");
    expect(formatWeight(800)).toBe("800 kg");
  });

  it("rounds waits into words", () => {
    expect(formatDuration(30, "uz")).toBe("30 son.");
    expect(formatDuration(3900, "ru")).toBe("1 ч 5 мин");
  });
});

describe("labels", () => {
  it("normalises phone numbers people type", () => {
    for (const typed of ["+998 90 123 45 67", "998901234567", "90 123-45-67"]) {
      expect(normalizePhone(typed)).toBe("+998901234567");
    }
    expect(UZ_PHONE.test(normalizePhone("12345"))).toBe(false);
  });

  it("explains guard reasons and Telegram codes", () => {
    expect(reasonText("cooldown", t)).toBe(uz["reason.cooldown"]);
    expect(reasonText("FLOOD_WAIT_600", t)).toBe(uz["reason.flood_wait"]);
    expect(reasonText("SOMETHING_NEW", t)).toBe("SOMETHING_NEW");
  });

  it("keeps the server's specifics under a translated title", () => {
    const quota = new ApiError(402, "QUOTA_EXCEEDED", "Your plan has 3 of 10 posts left.");
    expect(describeError(quota, t)).toBe(`${uz["err.QUOTA_EXCEEDED"]}. Your plan has 3 of 10 posts left.`);
    expect(describeError(new ApiError(0, "NETWORK", "network"), t)).toBe(uz["err.network"]);
    expect(describeError(new Error("boom"), t)).toBe(uz["err.generic"]);
  });
});

describe("advert form", () => {
  it("round-trips a parse draft into the request shape", () => {
    const draft = {
      origin_country: "UZ",
      origin_city_id: "c1",
      origin_region_id: "r1",
      origin_text: "Toshkentdan",
      destination_country: "RU",
      destination_text: "Moskvaga",
      weight_kg: 20000,
      price_amount: 250000,
      price_currency: "USD",
      contact_phone: "+998901234567",
    };
    const request = toRequest(fromDraft(draft));
    expect(request).toMatchObject({
      origin_country: "UZ",
      origin_city_id: "c1",
      origin_region_id: "r1",
      destination_country: "RU",
      destination_region_id: null,
      weight_kg: 20000,
      price_amount: 250000,
      price_currency: "USD",
      contact_phone: "+998901234567",
      vehicle_type: null,
    });
  });

  it("sends no currency without a price", () => {
    const values = fromDraft({});
    values.price_currency = "USD";
    expect(toRequest(values)).toMatchObject({ price_amount: null, price_currency: null });
  });
});

describe("session refresh", () => {
  const fetchMock = vi.fn();

  beforeEach(() => {
    localStorage.clear();
    fetchMock.mockReset();
    vi.stubGlobal("fetch", fetchMock);
  });
  afterEach(() => vi.unstubAllGlobals());

  const json = (status: number, body: unknown) =>
    new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });

  it("renews once however many requests ask at the same time", async () => {
    tokens.set({ access_token: "old", refresh_token: "r1", expires_at: "" });
    fetchMock.mockImplementation(async () =>
      json(200, { success: true, data: { user: {}, tokens: { access_token: "new", refresh_token: "r2" } } }),
    );

    const results = await Promise.all([refreshSession(), refreshSession(), refreshSession()]);

    expect(results).toEqual([true, true, true]);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(tokens.refresh()).toBe("r2");
  });

  it("retries a request after renewing an expired token", async () => {
    tokens.set({ access_token: "old", refresh_token: "r1", expires_at: "" });
    fetchMock
      .mockResolvedValueOnce(json(401, { success: false, error: { code: "TOKEN_EXPIRED", message: "expired" } }))
      .mockResolvedValueOnce(
        json(200, { success: true, data: { user: {}, tokens: { access_token: "new", refresh_token: "r2" } } }),
      )
      .mockResolvedValueOnce(json(200, { success: true, data: { ok: 1 } }));

    await expect(api.get<{ ok: number }>("/dashboard")).resolves.toEqual({ ok: 1 });
    const retried = fetchMock.mock.calls[2]![1] as RequestInit;
    expect((retried.headers as Record<string, string>).Authorization).toBe("Bearer new");
  });

  it("gives up and clears the session when renewal is refused", async () => {
    tokens.set({ access_token: "old", refresh_token: "r1", expires_at: "" });
    fetchMock
      .mockResolvedValueOnce(json(401, { success: false, error: { code: "TOKEN_EXPIRED", message: "expired" } }))
      .mockResolvedValueOnce(json(401, { success: false, error: { code: "TOKEN_REUSE_DETECTED", message: "reuse" } }))
      .mockResolvedValueOnce(json(401, { success: false, error: { code: "UNAUTHORIZED", message: "no" } }));

    await expect(api.get("/dashboard")).rejects.toMatchObject({ status: 401 });
    expect(tokens.refresh()).toBeNull();
  });
});
