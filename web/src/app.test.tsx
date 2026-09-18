import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterAll, beforeAll, describe, expect, it, vi } from "vitest";
import { App } from "./App";
import { uz } from "./i18n/uz";

// The whole signed-in path against a fake API: dashboard, adverts, writing an
// advert, saving it, reviewing the groups and publishing.

const AD_ID = "ad-1";
// Written the way people post in the groups: an emoji, lines, a phone number.
const AD_TEXT = "🚛 Toshkent → Moskva\n20 tonna tent, ertaga yuklash\nTel: +998 90 123 45 67";
const user = {
  id: "u-1",
  phone: "+998901234567",
  first_name: "Jalil",
  last_name: "Test",
  full_name: "Jalil Test",
  locale: "uz",
  status: "active",
  roles: ["company_manager"],
  permissions: ["load_ads.create", "load_ads.view", "load_ads.publish", "load_ads.delete", "posting.view", "dashboard.view", "telegram.connect", "telegram.groups.manage"],
  company_id: "c-1",
  company_role: "company_manager",
};

const ad = {
  id: AD_ID,
  status: "ready",
  raw_text: AD_TEXT,
  origin: { country: "UZ", text: "Toshkentdan" },
  destination: { country: "RU", text: "Moskvaga" },
  weight_kg: 20000,
  vehicle_type: "tent",
  price_amount: 250000,
  price_currency: "USD",
  price_negotiable: false,
  contact_phone: "+998901234567",
  missing: [],
  created_at: "2026-09-17T08:00:00Z",
  updated_at: "2026-09-17T08:00:00Z",
};

const dashboard = {
  scope: "company",
  day: "2026-09-17",
  timezone: "Asia/Tashkent",
  loads: { today: 3, month: 41 },
  posts: {
    today: { total: 12, queued: 2, sent: 9, failed: 1, skipped: 0, cancelled: 0 },
    month: { total: 120, queued: 2, sent: 100, failed: 8, skipped: 10, cancelled: 0 },
    success_rate: 0.9,
    last_sent_at: "2026-09-17T07:30:00Z",
  },
  guard_today: { cooldown: 4 },
  accounts: { total: 1, by_status: { active: 1 } },
  groups: { connected: 5, postable: 4 },
  ai: { enabled: false, requests_today: 0, requests_month: 0, cache_hits_month: 0, cost_month_usd: 0 },
  quota: [{ metric: "posts", limit: 200, used: 120, remaining: 80, resets_at: "2026-09-30T19:00:00Z" }],
  recent_problems: [
    { job_id: "j-9", group_id: "g-9", group_title: "Yuk markazi", status: "failed", reason: "FLOOD_WAIT_60", at: "2026-09-17T07:00:00Z" },
  ],
};

const calls: { method: string; path: string; body: unknown; headers: Record<string, string> }[] = [];

function reply(data: unknown, status = 200, meta?: unknown) {
  return new Response(JSON.stringify({ success: status < 400, data, meta }), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

async function fakeApi(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
  const url = new URL(String(input), "http://localhost");
  const method = init?.method ?? "GET";
  const path = url.pathname.replace("/api/v1", "");
  const body = init?.body ? JSON.parse(String(init.body)) : undefined;
  calls.push({ method, path, body, headers: (init?.headers ?? {}) as Record<string, string> });

  const route = `${method} ${path}`;
  switch (route) {
    case "GET /auth/me":
      return reply(user);
    case "GET /dashboard":
      return reply(dashboard);
    case "GET /notifications/unread-count":
      return reply({ notifications: 2, messages: 0 });
    case "GET /load-ads":
      return reply([ad], 200, { page: 1, per_page: 20, total: 1, total_pages: 1 });
    case "GET /reference/vehicle-types":
      return reply([{ code: "tent", name: "Tent" }]);
    case "GET /reference/cargo-types":
    case "GET /reference/regions":
    case "GET /reference/cities":
      return reply([]);
    case "GET /reference/countries":
      return reply([
        { id: "1", code: "UZ", name: "O‘zbekiston" },
        { id: "2", code: "RU", name: "Rossiya" },
      ]);
    case "POST /load-ads":
      return reply(ad, 201);
    case `GET /load-ads/${AD_ID}`:
      return reply(ad);
    case `GET /load-ads/${AD_ID}/posts`:
      return reply([]);
    case "GET /telegram/accounts":
      return reply([
        { id: "acc-1", user_id: "u-1", mine: true, phone: "+99890 *** ** 67", status: "active", created_at: "2026-09-01T00:00:00Z" },
      ]);
    case `POST /load-ads/${AD_ID}/publish/preview`:
      return reply({
        account_id: "acc-1",
        candidates: 2,
        approved: [{ group_id: "g-1", title: "Toshkent - Moskva yuklar", score: 55, reasons: ["route_country", "vehicle_listed"] }],
        skipped: [{ group_id: "g-2", title: "Fura bor", reason: "cooldown", retry_after_seconds: 1800 }],
        message_preview: AD_TEXT,
        preview_token: "tok-1",
        expires_at: new Date(Date.now() + 600_000).toISOString(),
        posts_left: 80,
      });
    case `POST /load-ads/${AD_ID}/publish`:
      return reply({ run_id: "r-1", load_ad_id: AD_ID, account_id: "acc-1", queued: 1, skipped: 0, jobs: [] }, 202);
    default:
      return reply(null, 404);
  }
}

beforeAll(() => {
  localStorage.setItem("loadify.access", "access");
  localStorage.setItem("loadify.refresh", "refresh");
  vi.stubGlobal("fetch", vi.fn(fakeApi));
  window.history.pushState({}, "", "/");
});

afterAll(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("signed-in flow", () => {
  it("goes from the dashboard to a published advert", async () => {
    render(<App />);

    // Dashboard
    expect(await screen.findByRole("heading", { level: 1, name: uz["dash.title"] })).toBeTruthy();
    expect(await screen.findByText("90%")).toBeTruthy();
    expect(screen.getByText("Yuk markazi")).toBeTruthy();
    expect(screen.getByText(uz["reason.flood_wait"])).toBeTruthy();
    expect(screen.getByText(uz["reason.cooldown"])).toBeTruthy();
    expect(screen.getByRole("link", { name: /Bildirishnomalar \(2\)/ })).toBeTruthy();

    // Adverts list: each advert is its first line
    fireEvent.click(screen.getAllByRole("link", { name: uz["nav.ads"] })[0]!);
    expect(await screen.findByRole("link", { name: "🚛 Toshkent → Moskva" })).toBeTruthy();

    // New advert: only the text, and an empty one is not saved
    fireEvent.click(screen.getAllByRole("link", { name: uz["ads.new"] })[0]!);
    const text = await screen.findByLabelText(uz["ed.textLabel"]);
    expect(screen.queryByRole("combobox")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: uz["common.save"] }));
    expect(await screen.findByText(uz["ed.textRequired"])).toBeTruthy();

    // Sent exactly as typed, spaces and lines included, and nothing else
    const typed = `  ${AD_TEXT}\n`;
    fireEvent.change(text, { target: { value: typed } });
    fireEvent.click(screen.getByRole("button", { name: uz["common.save"] }));
    await waitFor(() => expect(calls.some((c) => c.method === "POST" && c.path === "/load-ads")).toBe(true));
    const saved = calls.find((c) => c.method === "POST" && c.path === "/load-ads")!.body;
    expect(saved).toEqual({ raw_text: typed });
    expect(calls.some((c) => c.path === "/load-ads/parse")).toBe(false);

    // Detail: the text as it will be posted, then review and publish
    expect(await screen.findByText(uz["ad.text"])).toBeTruthy();
    const shown = (el: Element | null) => el?.tagName === "P" && el.textContent === AD_TEXT;
    expect(screen.getByText((_, el) => shown(el))).toBeTruthy();
    const review = await screen.findByRole("button", { name: uz["pub.review"] });
    fireEvent.click(review);
    expect(await screen.findByText("Toshkent - Moskva yuklar")).toBeTruthy();
    expect(screen.getByText("Fura bor")).toBeTruthy();
    expect(screen.getByText(/30 daq\./)).toBeTruthy();
    expect(screen.getByText(uz["match.vehicle_listed"])).toBeTruthy();
    expect(screen.getByText("Bu oy yana 80 ta post yuborish mumkin")).toBeTruthy();
    const preview = screen.getByText((_, el) => el?.tagName === "PRE" && el.textContent === AD_TEXT);
    expect(preview).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "1 ta guruhga yuborish" }));
    expect(await screen.findByText("1 ta post navbatga qo‘yildi")).toBeTruthy();

    const publish = calls.find((c) => c.path === `/load-ads/${AD_ID}/publish`)!;
    expect(publish.body).toEqual({ preview_token: "tok-1", group_ids: ["g-1"] });
    expect(publish.headers["Idempotency-Key"]).toMatch(/^[0-9a-f-]{36}$/);
    expect(publish.headers["Accept-Language"]).toBe("uz");
    expect(publish.headers.Authorization).toBe("Bearer access");

  });
});
