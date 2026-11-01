import { describe, expect, it } from "vitest";

import { consentSignals, scriptURL, syncGtm } from "@/app/lib/analytics/gtm";

const NONE = { analytics: false, ads: false, personalization: false };

function fakePage() {
  const scripts: string[] = [];
  const w = { dataLayer: undefined as unknown[] | undefined, addEventListener: () => {}, requestIdleCallback: (cb: () => void) => cb() } as unknown as Window & { dataLayer: unknown[] };
  const doc = {
    readyState: "complete",
    createElement: () => ({}) as HTMLScriptElement,
    head: { appendChild: (s: HTMLScriptElement) => scripts.push(s.src) },
  } as unknown as Document;
  return { w, doc, scripts };
}

const commands = (dl: unknown[]) => dl.filter((e) => typeof e === "object" && e !== null && "length" in e).map((a) => Array.from(a as ArrayLike<unknown>));

describe("the web GTM container", () => {
  it("loads nothing before the visitor allows a category, or without a valid container", () => {
    const { w, doc, scripts } = fakePage();
    expect(syncGtm(w, doc, "GTM-ABC123", "", NONE)).toBe(false);
    expect(syncGtm(w, doc, "not-a-container", "", { ...NONE, analytics: true })).toBe(false);
    expect(scripts).toEqual([]);
    expect(w.dataLayer).toBeUndefined();
  });

  it("declares consent denied, then the visitor's choice, before gtm.js - once", () => {
    const { w, doc, scripts } = fakePage();
    expect(syncGtm(w, doc, "GTM-ABC123", "", { analytics: true, ads: false, personalization: false })).toBe(true);
    const cmds = commands(w.dataLayer);
    expect(cmds[0].slice(0, 2)).toEqual(["consent", "default"]);
    expect(cmds[0][2]).toMatchObject({ analytics_storage: "denied", ad_storage: "denied", wait_for_update: 500 });
    expect(cmds[1]).toEqual(["consent", "update", consentSignals({ analytics: true, ads: false, personalization: false })]);
    expect(scripts).toEqual(["https://www.googletagmanager.com/gtm.js?id=GTM-ABC123"]);

    syncGtm(w, doc, "GTM-ABC123", "", NONE); // withdrawn
    expect(commands(w.dataLayer).at(-1)).toEqual(["consent", "update", consentSignals(NONE)]);
    expect(scripts).toHaveLength(1);
  });

  it("serves gtm.js from a first-party server-side container when one is named, https only", () => {
    expect(scriptURL("GTM-ABC123", "https://tags.shop.example/")).toBe("https://tags.shop.example/gtm.js?id=GTM-ABC123");
    expect(scriptURL("GTM-ABC123", "http://tags.shop.example")).toBe("https://www.googletagmanager.com/gtm.js?id=GTM-ABC123");
    expect(scriptURL("GTM-ABC123", "javascript:alert(1)")).toBe("https://www.googletagmanager.com/gtm.js?id=GTM-ABC123");
  });
});
