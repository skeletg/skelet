import { expect, it, vi } from "vitest";

vi.mock("@/app/lib/locale/read", () => ({
  getCurrentChannelLocales: async () => ({ codes: ["en", "de"], defaultLocale: "en" }),
}));
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
