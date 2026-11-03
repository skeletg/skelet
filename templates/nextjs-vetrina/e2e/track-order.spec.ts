import { expect, test } from "@playwright/test";

import { main, open } from "./helpers";

// No e2e helper places an order, so the match paths live in the Go and action unit tests - this covers the page itself
test("an unknown order says so and keeps what was typed", async ({ page }) => {
  await open(page, "/track-order?number=ORD-999999999");
  const form = main(page);
  await expect(form.getByLabel("Order number")).toHaveValue("ORD-999999999");
  await form.getByLabel("Email").fill("nobody@example.invalid");
  await form.getByLabel("Postcode").fill("00000");
  await form.getByRole("button", { name: "Find my order" }).click();

  await expect(form.getByText(/We couldn't find an order with those details/)).toBeVisible();
  await expect(form.getByLabel("Email")).toHaveValue("nobody@example.invalid");
});
