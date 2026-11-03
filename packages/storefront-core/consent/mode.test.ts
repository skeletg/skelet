import { afterEach, describe, expect, it, vi } from "vitest";

import { getConsentMode } from "./mode";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("getConsentMode", () => {
  it("keeps an opt-out shop opt-out when the backend stops answering", async () => {
    vi.useFakeTimers();
    vi.stubGlobal("fetch", vi.fn(async () => Response.json({ data: { publicConfig: [{ path: "general/privacy/cookie_consent", value: "opt_out" }] } })));
    expect(await getConsentMode("ch-1")).toBe("opt_out");

    vi.advanceTimersByTime(61_000);
    vi.stubGlobal("fetch", vi.fn(async () => { throw new Error("backend down"); }));
    expect(await getConsentMode("ch-1")).toBe("opt_out");
  });

  it("asks first when it never had an answer", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response("", { status: 502 })));
    expect(await getConsentMode("ch-2")).toBe("opt_in");
  });
});
