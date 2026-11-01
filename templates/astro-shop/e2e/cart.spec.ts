import { expect, test } from "@playwright/test";

import { addToCart, openBuyableProduct, scripted } from "./helpers";

test("an empty cart says so and is kept out of the index", async ({ page }) => {
  await page.goto("/cart");
  await expect(page.getByText("Your cart is empty")).toBeVisible();
  await expect(page.locator('meta[name="robots"]')).toHaveAttribute("content", /noindex/);
});

test("add, change and remove a line", async ({ page }) => {
  await openBuyableProduct(page);
  const name = await addToCart(page);

  const line = () => page.locator("[data-line]").filter({ hasText: name });
  await expect(line()).toHaveCount(1);
  // The header badge is filled from a cookie by script, so it only counts with scripts on
  const badge = page.locator("[data-cart-count]");
  if (scripted()) await expect(badge).toHaveText("1");

  // Set on this document only - still there at the end means no edit below reloaded the page
  if (scripted()) await page.evaluate(() => ((window as { stayed?: boolean }).stayed = true));

  if (scripted()) {
    // Two quick taps settle into one update
    await line().getByRole("button", { name: `One more ${name}` }).click();
    await line().getByRole("button", { name: `One more ${name}` }).click();
  } else {
    await line().getByRole("spinbutton").fill("3");
    await line().getByRole("button", { name: "Update" }).click();
  }
  if (scripted()) await expect(badge).toHaveText("3");
  await expect(line().getByRole("spinbutton")).toHaveValue("3");

  // Clearing the box to retype must not delete the line - it's refused with a message instead
  await line().getByRole("spinbutton").fill("");
  if (scripted()) await line().getByRole("spinbutton").press("Tab");
  else await line().getByRole("button", { name: "Update" }).click();
  await expect(page.getByRole("alert")).toBeVisible();
  await expect(line()).toHaveCount(1);

  await line().getByRole("button", { name: "Remove" }).click();
  await expect(page.getByText("Your cart is empty")).toBeVisible();
  if (scripted()) await expect(badge).toBeHidden();

  if (scripted()) expect(await page.evaluate(() => (window as { stayed?: boolean }).stayed)).toBe(true);
});

test("the badge survives navigation back to a cached page", async ({ page }) => {
  test.skip(!scripted(), "the badge is filled by script");
  await openBuyableProduct(page);
  await addToCart(page);
  await page.goto("/");
  await expect(page.locator("[data-cart-count]")).toHaveText("1");
});
