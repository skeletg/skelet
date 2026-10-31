import { expect, test, type BrowserContext } from "@playwright/test";

// The store's default consent mode, Ask first: nothing optional is set until the visitor chooses, each category is its
// own choice, and the footer takes it back. Each test starts from an empty cookie jar

const consentCookie = async (context: BrowserContext) =>
  (await context.cookies()).find((c) => c.name === "storefront_consent")?.value;
const visitorCookie = async (context: BrowserContext) =>
  (await context.cookies()).find((c) => c.name === "lens_aid")?.value;
const banner = (page: import("@playwright/test").Page) => page.getByRole("region", { name: "Cookie consent" });

test("a first visit sets no visitor id and asks before anything optional", async ({ page, context }) => {
  await page.goto("/");
  await expect(banner(page)).toBeVisible();
  expect(await consentCookie(context)).toBeUndefined();
  expect(await visitorCookie(context)).toBeUndefined();
});

test("a visitor can allow analytics alone, and the visitor id follows on the next page", async ({ page, context }) => {
  await page.goto("/");
  await banner(page).getByRole("button", { name: "Customize" }).click();
  await banner(page).getByRole("checkbox", { name: /Analytics/ }).check();
  await banner(page).getByRole("button", { name: "Save choices" }).click();
  await expect(banner(page)).toBeHidden();
  expect(decodeURIComponent((await consentCookie(context)) ?? "")).toBe("v2:a1.d0.p0");

  await page.goto("/products");
  expect(await visitorCookie(context)).toBeTruthy();
});

test("withdrawing from the footer drops the visitor id", async ({ page, context }) => {
  await page.goto("/");
  await banner(page).getByRole("button", { name: "Accept all" }).click();
  await expect(banner(page)).toBeHidden();
  await page.goto("/products");
  expect(await visitorCookie(context)).toBeTruthy();

  await page.getByRole("button", { name: "Cookie settings" }).click();
  await banner(page).getByRole("button", { name: "Essential only" }).click();
  await expect(banner(page)).toBeHidden();
  expect(decodeURIComponent((await consentCookie(context)) ?? "")).toBe("v2:a0.d0.p0");

  await page.goto("/products");
  expect(await visitorCookie(context)).toBeUndefined();
});
