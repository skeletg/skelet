import { describe, expect, it } from "vitest";

import { initOptions, isMixpanelCookie, syncMixpanel, type ReplayLib } from "@/app/lib/analytics/mixpanel";

const TOKEN = "0123456789abcdef0123456789abcdef";
const cfg = { token: TOKEN, region: "eu", percent: 25 };

function fakeLib() {
  const calls: unknown[][] = [];
  const lib = new Proxy({} as ReplayLib, { get: (_, name) => (name === "then" ? undefined : (...args: unknown[]) => void calls.push([name, ...args])) });
  const removed: string[] = [];
  return { lib, calls, load: async () => lib, storage: { removeItem: (k: string) => void removed.push(k) }, removed };
}

const fresh = () => ({ lib: null, identified: null, on: false });

describe("Mixpanel session replay", () => {
  it("loads nothing without analytics consent or a valid token", async () => {
    const f = fakeLib();
    let loads = 0;
    const load = async () => (loads++, f.lib);
    expect(await syncMixpanel(cfg, false, null, f.storage, load, fresh())).toBe(false);
    expect(await syncMixpanel({ ...cfg, token: "nope" }, true, null, f.storage, load, fresh())).toBe(false);
    expect(loads).toBe(0);
  });

  it("records without sending events, against the project's region, joined to a signed-in customer", async () => {
    const f = fakeLib();
    expect(await syncMixpanel(cfg, true, "cust-1", f.storage, f.load, fresh())).toBe(true);
    expect(f.removed).toEqual([`__mp_opt_in_out_${TOKEN}`]);
    expect(f.calls[0]).toEqual(["init", TOKEN, initOptions(cfg)]);
    expect(f.calls[1]).toEqual(["identify", "cust-1"]);
    const opts = initOptions(cfg);
    expect(opts).toMatchObject({ api_host: "https://api-eu.mixpanel.com", autocapture: false, track_pageview: false, record_sessions_percent: 25, record_mask_all_inputs: true });
    expect(initOptions({ ...cfg, region: "mars", percent: 900 })).toMatchObject({ api_host: "https://api-js.mixpanel.com", record_sessions_percent: 100 });
  });

  it("starts a new device on sign-out, and stops and forgets on a withdrawal", async () => {
    const f = fakeLib();
    const state = fresh();
    await syncMixpanel(cfg, true, "cust-1", f.storage, f.load, state);
    await syncMixpanel(cfg, true, null, f.storage, f.load, state);
    expect(f.calls.at(-1)).toEqual(["reset"]);
    await syncMixpanel(cfg, false, null, f.storage, f.load, state);
    expect(f.calls.slice(-2)).toEqual([["stop_session_recording"], ["opt_out_tracking", { delete_user: false }]]);
    const before = f.calls.length;
    expect(await syncMixpanel(cfg, true, null, f.storage, f.load, state)).toBe(false); // back on the next page load, not this one
    expect(f.calls.length).toBe(before);
  });

  it("names its own cookie only", () => {
    expect(isMixpanelCookie(`mp_${TOKEN}_mixpanel`)).toBe(true);
    expect(isMixpanelCookie("mp_other")).toBe(false);
    expect(isMixpanelCookie("lens_aid")).toBe(false);
  });
});
