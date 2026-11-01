import { expect, it, vi } from "vitest";

let locales = { codes: ["en", "de"], defaultLocale: "en" };
vi.mock("./read", () => ({ getCurrentChannelLocales: async () => locales }));
const { withLocalePrefix } = await import("./path");

it("prefixes a non-default locale and keeps the query", async () => {
  expect(await withLocalePrefix("/account?done=redeem", "de")).toBe("/de/account?done=redeem");
  expect(await withLocalePrefix("/?done=1", "de")).toBe("/de?done=1");
  expect(await withLocalePrefix("/", "de")).toBe("/de");
});

it("never stacks a second prefix, query or not", async () => {
  expect(await withLocalePrefix("/de/checkout?step=payment", "de")).toBe("/de/checkout?step=payment");
  expect(await withLocalePrefix("/de?welcome=1", "de")).toBe("/de?welcome=1");
  expect(await withLocalePrefix("/de#top", "de")).toBe("/de#top");
});

it("leaves the default locale and unknown locales unprefixed", async () => {
  expect(await withLocalePrefix("/account?x=1", "en")).toBe("/account?x=1");
  expect(await withLocalePrefix("/account", "fr")).toBe("/account");
});

it("puts nothing but a locale code in front of a path", async () => {
  locales = { codes: [], defaultLocale: "" };
  for (const bad of ["/evil.com", "/", "", "de/../x", "de?x", "de\\x", "DE", "d"]) {
    expect(await withLocalePrefix("/checkout", bad)).toBe("/checkout");
  }
  expect(await withLocalePrefix("/checkout", "pt-BR")).toBe("/pt-BR/checkout");
});
