import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { uz } from "./i18n/uz";

// Registration is one form: company, phone, password. No SMS step unless the
// server asks for one.

const newUser = {
  id: "u-2",
  phone: "+998901112233",
  first_name: "",
  last_name: "",
  full_name: "",
  locale: "uz",
  status: "active",
  roles: ["company_manager"],
  permissions: ["dashboard.view", "load_ads.create"],
};

const dashboard = {
  scope: "company",
  day: "2026-09-18",
  timezone: "Asia/Tashkent",
  loads: { today: 0, month: 0 },
  posts: {
    today: { total: 0, queued: 0, sent: 0, failed: 0, skipped: 0, cancelled: 0 },
    month: { total: 0, queued: 0, sent: 0, failed: 0, skipped: 0, cancelled: 0 },
    success_rate: null,
    last_sent_at: null,
  },
  guard_today: {},
  accounts: { total: 0, by_status: {} },
  groups: { connected: 0, postable: 0 },
  ai: { enabled: false, requests_today: 0, requests_month: 0, cache_hits_month: 0, cost_month_usd: 0 },
  quota: [],
  recent_problems: [],
};

const reply = (data: unknown, status = 200) =>
  new Response(JSON.stringify({ success: status < 400, data }), {
    status,
    headers: { "Content-Type": "application/json" },
  });

function fakeApi(verificationRequired: boolean) {
  const calls: { route: string; body: unknown; auth?: string }[] = [];
  let hasCompany = false;
  const fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), "http://localhost");
    const route = `${init?.method ?? "GET"} ${url.pathname.replace("/api/v1", "")}`;
    const body = init?.body ? JSON.parse(String(init.body)) : undefined;
    calls.push({ route, body, auth: (init?.headers as Record<string, string>)?.Authorization });
    const tokens = (n: number) => ({ access_token: `access-${n}`, refresh_token: `refresh-${n}`, expires_at: "" });

    switch (route) {
      case "POST /auth/register":
        return verificationRequired
          ? reply({ user_id: "u-2", phone: body.phone, verification_required: true }, 201)
          : reply({ user_id: "u-2", phone: body.phone, verification_required: false, auth: { user: newUser, tokens: tokens(1) } }, 201);
      case "POST /companies":
        hasCompany = true;
        return reply({ id: "c-2", legal_name: body.legal_name, brand_name: body.brand_name, status: "pending" }, 201);
      case "POST /auth/refresh":
        return reply({ user: newUser, tokens: tokens(2) });
      case "GET /auth/me":
        return reply(hasCompany ? { ...newUser, company_id: "c-2", company_role: "company_manager" } : newUser);
      case "GET /dashboard":
        return reply(dashboard);
      case "GET /notifications/unread-count":
        return reply({ notifications: 0, messages: 0 });
      default:
        return reply(null, 404);
    }
  });
  return { fetch, calls };
}

beforeEach(() => {
  localStorage.clear();
  window.history.pushState({}, "", "/register");
  // The router reads the address when the app module loads, so each test
  // loads it afresh after setting the address.
  vi.resetModules();
});

async function renderApp() {
  const { App } = await import("./App");
  render(<App />);
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function fillAndSubmit() {
  fireEvent.change(screen.getByLabelText(uz["auth.companyName"]), { target: { value: "Yuk Express" } });
  fireEvent.change(screen.getByLabelText(uz["auth.phone"]), { target: { value: "90 111 22 33" } });
  fireEvent.change(screen.getByLabelText(uz["auth.password"]), { target: { value: "Loadify2026x" } });
  fireEvent.click(screen.getByRole("button", { name: uz["auth.register.submit"] }));
}

describe("registration", () => {
  it("registers, creates the company and opens the dashboard in one step", async () => {
    const api = fakeApi(false);
    vi.stubGlobal("fetch", api.fetch);
    await renderApp();

    expect(await screen.findByRole("heading", { name: uz["auth.register.title"] })).toBeTruthy();
    expect(screen.queryByLabelText(uz["auth.firstName"])).toBeNull();
    fillAndSubmit();

    expect(await screen.findByRole("heading", { level: 1, name: uz["dash.title"] })).toBeTruthy();
    expect(screen.queryByText(uz["auth.verify.title"])).toBeNull();
    expect(screen.queryByText(uz["onb.title"])).toBeNull();

    const routes = api.calls.map((c) => c.route);
    expect(routes.slice(0, 4)).toEqual(["POST /auth/register", "POST /companies", "POST /auth/refresh", "GET /auth/me"]);
    expect(api.calls[0]!.body).toEqual({ phone: "+998901112233", password: "Loadify2026x", locale: "uz", role: "company" });
    expect(api.calls[1]!.body).toEqual({ legal_name: "Yuk Express", brand_name: "Yuk Express", phone: "+998901112233" });
    expect(api.calls[1]!.auth).toBe("Bearer access-1");
    expect(api.calls[3]!.auth).toBe("Bearer access-2");
    // Registered without a name, the person is shown by phone.
    expect(screen.getAllByText("+998901112233").length).toBeGreaterThan(0);
  });

  it("asks for the SMS code only when the server requires it", async () => {
    const api = fakeApi(true);
    vi.stubGlobal("fetch", api.fetch);
    await renderApp();

    await screen.findByRole("heading", { name: uz["auth.register.title"] });
    fillAndSubmit();

    expect(await screen.findByText(uz["auth.verify.title"])).toBeTruthy();
    await waitFor(() => expect(api.calls.some((c) => c.route === "POST /companies")).toBe(false));
  });
});
