import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { uz } from "./i18n/uz";

// The plan page: what the company is on now, what it has used, and what it
// could move to. Buying ends at the provider's page, never with card details
// typed here.

const user = {
  id: "u-1",
  phone: "+998901234567",
  first_name: "Jalil",
  last_name: "Test",
  full_name: "Jalil Test",
  locale: "uz",
  status: "active",
  roles: ["company_owner"],
  permissions: ["dashboard.view", "payments.manage", "payments.read"],
  company_id: "c-1",
  company_role: "company_owner",
};

const START = {
  id: "plan-start",
  code: "company_start",
  name: "Start",
  target_role: "company",
  price_amount: 0,
  currency: "UZS",
  period: "month",
  trial_days: 0,
  features: [],
  limits: { tg_accounts: 1, tg_groups: 15, loads: 30, posts: 200, ai_requests: 50 },
  is_default: true,
  is_current: true,
};

const BUSINESS = {
  ...START,
  id: "plan-business",
  code: "company_business_m",
  name: "Business (oylik)",
  price_amount: 29900000,
  trial_days: 14,
  limits: { tg_accounts: 5, tg_groups: 150, loads: 1000, posts: 10000, ai_requests: 3000 },
  is_default: false,
  is_current: false,
};

const ENTERPRISE = {
  ...BUSINESS,
  id: "plan-enterprise",
  code: "company_enterprise_m",
  name: "Enterprise",
  price_amount: 99900000,
  trial_days: 0,
  limits: { tg_accounts: -1, tg_groups: -1, loads: -1, posts: -1, ai_requests: -1 },
};

const usage = {
  period_start: "2026-09-01T00:00:00Z",
  period_end: "2026-10-01T00:00:00Z",
  items: [
    { metric: "posts", limit: 200, used: 120, remaining: 80, resets_at: "2026-09-30T19:00:00Z" },
    { metric: "tg_groups", limit: 15, used: 4, remaining: 11 },
  ],
};

const reply = (data: unknown, status = 200) =>
  new Response(JSON.stringify({ success: status < 400, data }), {
    status,
    headers: { "Content-Type": "application/json" },
  });

function fakeApi(options: { subscription?: unknown; checkout?: unknown } = {}) {
  const calls: { route: string; body: unknown }[] = [];
  const fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), "http://localhost");
    const route = `${init?.method ?? "GET"} ${url.pathname.replace("/api/v1", "")}`;
    calls.push({ route, body: init?.body ? JSON.parse(String(init.body)) : undefined });

    switch (route) {
      case "GET /auth/me":
        return reply(user);
      case "GET /notifications/unread-count":
        return reply({ notifications: 0, messages: 0 });
      case "GET /subscriptions/usage":
        return reply(usage);
      case "GET /plans":
        return reply([START, BUSINESS, ENTERPRISE]);
      case "GET /subscriptions/me":
        return reply(options.subscription ?? { subscription: null });
      case "GET /payments/methods":
        return reply({ providers: ["click", "payme", "wallet"] });
      case "POST /subscriptions":
        return reply(options.checkout ?? { order_id: "o-1", provider: "wallet", amount: 29900000, currency: "UZS", status: "paid", paid: true }, 201);
      case "POST /subscriptions/cancel":
        return reply({ message: "Auto-renewal turned off" });
      default:
        return reply(null, 404);
    }
  });
  return { fetch, calls };
}

beforeEach(() => {
  localStorage.clear();
  localStorage.setItem("loadify.access", "access");
  localStorage.setItem("loadify.refresh", "refresh");
  window.history.pushState({}, "", "/usage");
  // The router reads the address when the app module loads.
  vi.resetModules();
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

async function renderApp() {
  const { App } = await import("./App");
  render(<App />);
}

describe("plans", () => {
  it("shows the free plan, what it allows and what it has used", async () => {
    vi.stubGlobal("fetch", fakeApi().fetch);
    await renderApp();

    expect(await screen.findByRole("heading", { level: 1, name: uz["usage.title"] })).toBeTruthy();

    // The free tier is not a subscription row; the default plan is the answer.
    const currentCard = await screen.findByRole("region", { name: uz["plan.currentTitle"] });
    expect(within(currentCard).getByText("Start")).toBeTruthy();
    expect(within(currentCard).getByText(uz["plan.freeForever"])).toBeTruthy();
    expect(within(currentCard).queryByRole("button", { name: uz["plan.stopRenewal"] })).toBeNull();

    // What has been used this month.
    expect(screen.getByText("120 / 200")).toBeTruthy();

    // What each plan would give.
    const business = await screen.findByRole("region", { name: "Business (oylik)" });
    expect(within(business).getByText("299 000 UZS / oyiga")).toBeTruthy();
    expect(within(business).getByText(uz["plan.trial"].replace("{n}", "14"))).toBeTruthy();
    expect(within(business).getByText("10000")).toBeTruthy();

    const start = screen.getByRole("region", { name: "Start" });
    expect(within(start).getByRole("button", { name: uz["plan.current"] }).hasAttribute("disabled")).toBe(true);

    const enterprise = screen.getByRole("region", { name: "Enterprise" });
    expect(within(enterprise).getAllByText(uz["common.unlimited"])).toHaveLength(5);
  });

  it("buys a plan through a provider and never asks for card details", async () => {
    const api = fakeApi({
      checkout: { order_id: "o-1", provider: "click", amount: 29900000, currency: "UZS", status: "pending", paid: false },
    });
    vi.stubGlobal("fetch", api.fetch);
    const assigned: string[] = [];
    vi.stubGlobal("location", { ...window.location, origin: "http://localhost:3000", set href(url: string) { assigned.push(url); } });
    await renderApp();

    const business = await screen.findByRole("region", { name: "Business (oylik)" });
    fireEvent.click(within(business).getByRole("button", { name: uz["plan.buy"] }));
    expect(await screen.findByText(uz["plan.checkoutHint"])).toBeTruthy();
    expect(screen.queryByLabelText(/card|karta/i)).toBeNull();

    fireEvent.change(await screen.findByLabelText(uz["plan.method"]), { target: { value: "payme" } });
    fireEvent.click(screen.getByRole("button", { name: uz["plan.toPayment"] }));

    await waitFor(() => expect(api.calls.some((c) => c.route === "POST /subscriptions")).toBe(true));
    expect(api.calls.find((c) => c.route === "POST /subscriptions")!.body).toEqual({
      plan_id: "plan-business",
      provider: "payme",
      auto_renew: true,
      return_url: "http://localhost:3000/usage",
    });
  });

  it("turns off auto-renewal after confirming", async () => {
    const api = fakeApi({
      subscription: {
        id: "sub-1",
        plan: BUSINESS,
        status: "active",
        expires_at: "2026-10-18T00:00:00Z",
        auto_renew: true,
        created_at: "2026-09-18T00:00:00Z",
      },
    });
    vi.stubGlobal("fetch", api.fetch);
    await renderApp();

    expect(await screen.findByText(uz["plan.status.active"])).toBeTruthy();
    expect(screen.getByText(uz["plan.renews"].replace("{date}", "18.10.2026"))).toBeTruthy();

    const currentCard = screen.getByRole("region", { name: uz["plan.currentTitle"] });
    fireEvent.click(within(currentCard).getByRole("button", { name: uz["plan.stopRenewal"] }));

    // Confirming is a second, deliberate click, inside the dialog.
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText(uz["plan.stopRenewalConfirm"])).toBeTruthy();
    fireEvent.click(within(dialog).getByRole("button", { name: uz["plan.stopRenewal"] }));

    await waitFor(() => expect(api.calls.some((c) => c.route === "POST /subscriptions/cancel")).toBe(true));
    expect(await screen.findByText(uz["plan.renewalStopped"])).toBeTruthy();
  });
});
