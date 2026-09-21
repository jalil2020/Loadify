import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { uz } from "./i18n/uz";

// An empty review has two causes that need different answers: no group is
// connected yet, or the connected ones did not fit this advert. Telling
// someone to check rules they never wrote sends them looking in the wrong place.

const AD_ID = "ad-1";

const user = {
  id: "u-1",
  phone: "+998901234567",
  first_name: "",
  last_name: "",
  full_name: "",
  locale: "uz",
  status: "active",
  roles: ["company_manager"],
  permissions: ["load_ads.view", "load_ads.publish", "posting.view", "telegram.groups.manage"],
  company_id: "c-1",
  company_role: "company_manager",
};

const ad = {
  id: AD_ID,
  status: "ready",
  raw_text: "🚛 Toshkent → Moskva\n20 t tent",
  origin: { country: "UZ", text: "Toshkent" },
  destination: { country: "RU", text: "Moskva" },
  price_negotiable: false,
  missing: [],
  created_at: "2026-09-21T08:00:00Z",
  updated_at: "2026-09-21T08:00:00Z",
};

const group = {
  id: "g-1",
  account_id: "acc-1",
  peer_id: "-1001",
  title: "Yuk markazi",
  kind: "supergroup",
  language: "uz",
  slowmode_seconds: 0,
  can_send: true,
  posting_enabled: true,
  active: true,
};

const reply = (data: unknown, status = 200) =>
  new Response(JSON.stringify({ success: status < 400, data }), {
    status,
    headers: { "Content-Type": "application/json" },
  });

function fakeApi(groups: unknown[]) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), "http://localhost");
    const route = `${init?.method ?? "GET"} ${url.pathname.replace("/api/v1", "")}`;
    switch (route) {
      case "GET /auth/me":
        return reply(user);
      case "GET /notifications/unread-count":
        return reply({ notifications: 0, messages: 0 });
      case `GET /load-ads/${AD_ID}`:
        return reply(ad);
      case `GET /load-ads/${AD_ID}/posts`:
        return reply([]);
      case "GET /telegram/accounts":
        return reply([
          { id: "acc-1", user_id: "u-1", mine: true, phone: "+99890 *** ** 67", status: "active", created_at: "2026-09-01T00:00:00Z" },
        ]);
      case "GET /telegram/groups":
        return reply(groups);
      case `POST /load-ads/${AD_ID}/publish/preview`:
        // Nothing matched, whatever the reason.
        return reply({
          account_id: "acc-1",
          candidates: 0,
          approved: [],
          skipped: [],
          message_preview: ad.raw_text,
          preview_token: "tok-1",
          expires_at: new Date(Date.now() + 600_000).toISOString(),
          posts_left: 200,
        });
      default:
        return reply(null, 404);
    }
  });
}

beforeEach(() => {
  localStorage.clear();
  localStorage.setItem("loadify.access", "access");
  localStorage.setItem("loadify.refresh", "refresh");
  window.history.pushState({}, "", `/ads/${AD_ID}`);
  vi.resetModules();
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

async function review() {
  const { App } = await import("./App");
  render(<App />);
  fireEvent.click(await screen.findByRole("button", { name: uz["pub.review"] }));
}

describe("a review that finds nothing", () => {
  it("says to connect a group when there are none", async () => {
    vi.stubGlobal("fetch", fakeApi([]));
    await review();

    expect(await screen.findByText(uz["pub.noGroups"])).toBeTruthy();
    expect(screen.getByRole("link", { name: uz["pub.connectGroup"] }).getAttribute("href")).toBe("/telegram");
    expect(screen.queryByText(uz["pub.noCandidates"])).toBeNull();
  });

  it("points at the rules when groups are connected but none fit", async () => {
    vi.stubGlobal("fetch", fakeApi([group]));
    await review();

    expect(await screen.findByText(uz["pub.noCandidates"])).toBeTruthy();
    expect(screen.queryByText(uz["pub.noGroups"])).toBeNull();
  });
});
