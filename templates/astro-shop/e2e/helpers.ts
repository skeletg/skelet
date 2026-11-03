import { expect, test, type Page } from "@playwright/test";

// The dev catalog changes, so specs never name a product - they take the first one that can be bought
export async function openBuyableProduct(page: Page): Promise<string> {
  await page.goto("/");
  const hrefs = await page.locator("#products a[href^='/']").evaluateAll((as) => as.map((a) => a.getAttribute("href")!));
  for (const href of hrefs) {
    await page.goto(href);
    if (await page.getByRole("button", { name: "Add to cart" }).isEnabled().catch(() => false)) return href;
  }
  throw new Error("No product in the dev catalog can be added to a cart - seed one that's in stock");
}

// Whether this project runs with scripts - the enhanced forms (enhance.ts) behave differently then
export function scripted(): boolean {
  return test.info().project.use.javaScriptEnabled !== false;
}

// Adds the open product and ends on the cart page, whichever way the add went
export async function addToCart(page: Page): Promise<string> {
  const name = (await page.getByRole("heading", { level: 1 }).textContent())!.trim();
  await page.getByRole("button", { name: "Add to cart" }).click();
  if (scripted()) {
    // The add posted in the background - the product page stays, says so, and the badge counts it
    await expect(page.getByRole("status")).toHaveText("Added to your cart.");
    await expect(page).not.toHaveURL(/\/cart$/);
    await expect(page.locator("[data-cart-count]")).toBeVisible();
    await page.locator("header").getByRole("link", { name: /Cart/ }).click();
  }
  await expect(page).toHaveURL(/\/cart$/);
  return name;
}

export function jsonLd(page: Page) {
  return page
    .locator('script[type="application/ld+json"]')
    .evaluateAll((els) => els.map((e) => JSON.parse(e.textContent ?? "null") as { "@type": string; [k: string]: unknown }));
}
