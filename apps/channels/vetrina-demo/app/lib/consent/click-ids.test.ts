import { describe, expect, it } from "vitest";

import { clickCookieWrites, clickIdsHeader, cookieDomain } from "@/app/lib/consent/click-ids";

const jar = (cookies: Record<string, string>) => (name: string) => cookies[name];
const writes = (query: string, cookies: Record<string, string>, ads: boolean) =>
  clickCookieWrites(new URLSearchParams(query), jar(cookies), ads, "www.shop.example:3000", true, 1700000000000);
const byName = (w: ReturnType<typeof writes>) => Object.fromEntries(w.map((x) => [x.name, x]));

describe("click id cookies", () => {
  it("stores nothing before advertising consent, and a click that lands then is lost", () => {
    expect(writes("gclid=abc&cjevent=123&fbclid=xyz", {}, false)).toEqual([]);
  });

  it("deletes every stored click id once advertising consent is withdrawn", () => {
    const w = byName(writes("gclid=new", { cje: "1", awc: "2", _fbp: "fb.1.1.1", other: "x" }, false));
    expect(Object.keys(w).sort()).toEqual(["_fbp", "awc", "cje"]);
    for (const x of Object.values(w)) expect([x.value, x.options.maxAge]).toEqual(["", 0]);
  });

  it("sets CJ's cje the way its spec asks", () => {
    const { cje } = byName(writes("cjevent=abc123", {}, true));
    expect(cje).toEqual({
      name: "cje",
      value: "abc123",
      options: { path: "/", httpOnly: true, sameSite: "lax", secure: true, maxAge: 395 * 86400, domain: "shop.example" },
    });
  });

  it("overwrites on a new click and keeps the stored value without one", () => {
    expect(byName(writes("cjevent=second", { cje: "first" }, true)).cje.value).toBe("second");
    expect(byName(writes("", { cje: "first", _fbp: "fb.1.1.1" }, true))).toEqual({});
  });

  it("mints _fbp once and builds _fbc from fbclid, keeping its first-seen time", () => {
    const first = byName(writes("fbclid=Iw_1", {}, true));
    expect(first._fbp.value).toMatch(/^fb\.1\.1700000000000\.\d+$/);
    expect(first._fbc.value).toBe("fb.1.1700000000000.Iw_1");
    expect(first._fbc.options.httpOnly).toBe(false);
    expect(byName(writes("fbclid=Iw_1", { _fbp: "x", _fbc: "fb.1.5.Iw_1" }, true))).toEqual({});
  });

  it("refuses a value that is not a click id", () => {
    expect(byName(writes("gclid=" + encodeURIComponent("a;b=c") + "&awc=" + "x".repeat(256), { _fbp: "x" }, true))).toEqual({});
  });

  it("forwards the stored ids under their own names", () => {
    const h = new URLSearchParams(clickIdsHeader(jar({ sk_gclid: "g1", cje: "c1", _fbc: "fb.1.5.Iw", junk: "x" })));
    expect(Object.fromEntries(h)).toEqual({ gclid: "g1", cjevent: "c1", fbc: "fb.1.5.Iw" });
  });

  it("scopes to the parent of www. and leaves local hosts host-only", () => {
    expect(cookieDomain("www.Shop.example")).toBe("shop.example");
    expect(cookieDomain("eu.shop.example")).toBe("eu.shop.example");
    for (const h of ["localhost:3000", "127.0.0.1", "[::1]:3000"]) expect(cookieDomain(h)).toBeUndefined();
  });
});
