import { describe, expect, it } from "vitest";

import { clickCookieWrites, clickIdsHeader, cookieDomain } from "./click-ids";

const jar = (cookies: Record<string, string>) => (name: string) => cookies[name];
const writes = (query: string, cookies: Record<string, string>, ads: boolean) =>
  clickCookieWrites(new URLSearchParams(query), jar(cookies), ads, "www.shop.example:3000", true, 1700000000000);
const byName = (w: ReturnType<typeof writes>) => Object.fromEntries(w.map((x) => [x.name, x]));

describe("click id cookies", () => {
  it("stores nothing before advertising consent, and a click that lands then is lost", () => {
    expect(writes("gclid=abc&fbclid=xyz", {}, false)).toEqual([]);
  });

  it("deletes every stored click id once advertising consent is withdrawn", () => {
    const w = byName(writes("gclid=new", { sk_gclid: "1", sk_msclkid: "2", _fbp: "fb.1.1.1", other: "x" }, false));
    expect(Object.keys(w).sort()).toEqual(["_fbp", "sk_gclid", "sk_msclkid"]);
    for (const x of Object.values(w)) expect([x.value, x.options.maxAge]).toEqual(["", 0]);
  });

  it("sets a click id on the parent domain, HttpOnly", () => {
    const { sk_gclid } = byName(writes("gclid=abc123", {}, true));
    expect(sk_gclid).toEqual({
      name: "sk_gclid",
      value: "abc123",
      options: { path: "/", httpOnly: true, sameSite: "lax", secure: true, maxAge: 90 * 86400, domain: "shop.example" },
    });
  });

  it("overwrites on a new click and keeps the stored value without one", () => {
    expect(byName(writes("gclid=second", { sk_gclid: "first" }, true)).sk_gclid.value).toBe("second");
    expect(byName(writes("", { sk_gclid: "first", _fbp: "fb.1.1.1" }, true))).toEqual({});
  });


  it("mints _fbp once and builds _fbc from fbclid, keeping its first-seen time", () => {
    const first = byName(writes("fbclid=Iw_1", {}, true));
    expect(first._fbp.value).toMatch(/^fb\.1\.1700000000000\.\d+$/);
    expect(first._fbc.value).toBe("fb.1.1700000000000.Iw_1");
    expect(first._fbc.options.httpOnly).toBe(true);
    expect(byName(writes("fbclid=Iw_1", { _fbp: "x", _fbc: "fb.1.5.Iw_1" }, true))).toEqual({});
  });

  it("refuses a value that is not a click id", () => {
    expect(byName(writes("gclid=" + encodeURIComponent("a;b=c") + "&msclkid=" + "x".repeat(256), { _fbp: "x" }, true))).toEqual({});
  });

  it("forwards the stored ids under their own names", () => {
    const h = new URLSearchParams(clickIdsHeader(jar({ sk_gclid: "g1", cje: "c1", _fbc: "fb.1.5.Iw", junk: "x" }), true));
    expect(Object.fromEntries(h)).toEqual({ gclid: "g1", fbc: "fb.1.5.Iw" });
    expect(clickIdsHeader(jar({ sk_gclid: "g1" }), false)).toBe("");
  });

  it("scopes to the parent of www. and leaves local hosts host-only", () => {
    expect(cookieDomain("www.Shop.example", "")).toBe("shop.example");
    expect(cookieDomain("eu.shop.example", "")).toBe("eu.shop.example");
    for (const h of ["localhost:3000", "127.0.0.1", "[::1]:3000"]) expect(cookieDomain(h, "")).toBeUndefined();
  });

  it("takes the configured domain when the shop names one", () => {
    expect(cookieDomain("checkout.shop.co.uk", ".shop.co.uk")).toBe("shop.co.uk");
  });
});

describe("OpenAI's click ids", () => {
  it("stores oppref like any click, and only forwards the pixel's own __obref", () => {
    const w = byName(writes("oppref=op_1", {}, true));
    expect(w.sk_oppref.value).toBe("op_1");
    expect(w.__obref).toBeUndefined();
    expect(byName(writes("", { __obref: "123e4567-e89b-42d3-a456-426614174000", _fbp: "x" }, true))).toEqual({});
    const h = new URLSearchParams(clickIdsHeader(jar({ sk_oppref: "op_1", __obref: "123e4567-e89b-42d3-a456-426614174000" }), true));
    expect(Object.fromEntries(h)).toEqual({ oppref: "op_1", obref: "123e4567-e89b-42d3-a456-426614174000" });
  });

  it("deletes the pixel's __obref too once advertising consent is withdrawn", () => {
    const w = byName(writes("", { __obref: "x" }, false));
    expect([w.__obref.value, w.__obref.options.maxAge]).toEqual(["", 0]);
  });
});
