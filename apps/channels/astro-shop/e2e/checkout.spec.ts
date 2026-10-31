import { expect, test } from "@playwright/test";

import { addToCart, openBuyableProduct, scripted } from "./helpers";

// Runs twice - with JavaScript (chromium) and without (no-js) - because every step is a plain form

test("checkout without a cart goes back to the cart", async ({ page }) => {
  await page.goto("/checkout");
  await expect(page).toHaveURL(/\/cart$/);
});

test("place an order end to end", async ({ page }) => {
  await openBuyableProduct(page);
  const name = await addToCart(page);
  await page.getByRole("link", { name: "Checkout" }).click();
  await expect(page).toHaveURL(/\/checkout$/);
  await expect(page.locator('meta[name="robots"]')).toHaveAttribute("content", /noindex/);

  await page.getByLabel("Country").selectOption("US");
  await page.getByLabel("Postal code").fill("10001");
  await page.getByRole("button", { name: "Continue" }).click();

  // The delivery and payment choices come preselected - the first of each
  await expect(page.getByRole("heading", { name: "Delivery" })).toBeVisible();
  await expect(page.locator('input[name^="delivery:"]:checked')).toHaveCount(await page.locator('input[name="group"]').count());
  await expect(page.locator('input[name="payment"]:checked')).toHaveCount(1);

  const email = `e2e-astro-${Date.now()}@example.com`;
  await page.getByLabel("Email", { exact: true }).fill(email);
  await page.getByLabel("First name").fill("Ada");
  await page.getByLabel("Last name").fill("Astro");
  await page.getByLabel("Address", { exact: true }).fill("1 Main St");
  await page.getByLabel("City").fill("New York");
  await page.getByLabel("State / region").fill("NY");
  await page.getByLabel("I accept the terms and conditions").check();
  await expect(page.getByRole("complementary", { name: "Order summary" })).toContainText(name);

  await page.getByRole("button", { name: "Place order" }).click();
  await expect(page).toHaveURL(/\/order-placed\?n=/);
  await expect(page.getByRole("heading", { name: "Thank you for your order" })).toBeVisible();
  await expect(page.locator("[data-order-number]")).toHaveText(/\S+/);

  // The cart is spent
  await page.goto("/cart");
  await expect(page.getByText("Your cart is empty")).toBeVisible();
});

test("a backend refusal comes back as a message, with the form still filled in", async ({ page }) => {
  await openBuyableProduct(page);
  await addToCart(page);
  await page.goto("/checkout");
  await page.getByLabel("Country").selectOption("US");
  await page.getByLabel("Postal code").fill("10001");
  await page.getByRole("button", { name: "Continue" }).click();

  // An address the backend refuses but the browser's own validation lets through
  await page.getByLabel("Email", { exact: true }).fill("e2e-astro@example.com");
  await page.getByLabel("First name").fill("Ada");
  await page.getByLabel("Last name").fill("Astro");
  await page.getByLabel("Address", { exact: true }).fill(" ");
  await page.getByLabel("City").fill("New York");
  await page.getByLabel("I accept the terms and conditions").check();
  await page.getByRole("button", { name: "Place order" }).click();

  await expect(page.getByRole("alert")).toBeVisible();
  await expect(page).toHaveURL(/\/checkout$/);
  await expect(page.getByLabel("Email", { exact: true })).toHaveValue("e2e-astro@example.com");
});

test("picking another delivery option reprices the summary and keeps what was typed", async ({ page }) => {
  await openBuyableProduct(page);
  await addToCart(page);
  await page.goto("/checkout");
  await page.getByLabel("Country").selectOption("US");
  await page.getByLabel("Postal code").fill("10001");
  await page.getByRole("button", { name: "Continue" }).click();

  const options = page.locator('input[name^="delivery:"]');
  test.skip((await options.count()) < 2, "the dev catalog offers one delivery option to this address - nothing to switch to");
  await page.getByLabel("Email", { exact: true }).fill("e2e-astro@example.com");
  const unpicked = page.locator('input[name^="delivery:"]:not(:checked)').first();
  const value = await unpicked.getAttribute("value");
  await unpicked.check();
  if (!scripted()) await page.getByRole("button", { name: "Update shipping" }).click();

  await expect(page.locator(`input[name^="delivery:"][value="${value}"]`)).toBeChecked();
  await expect(page.getByLabel("Email", { exact: true })).toHaveValue("e2e-astro@example.com");
  await expect(page).toHaveURL(/\/checkout$/);
});
