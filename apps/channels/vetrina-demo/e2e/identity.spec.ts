import { expect, test } from "@playwright/test";
import { main, PASSWORD, registerCustomer } from "./helpers";

test("sidebar identity follows registration, logout and login without reloading", async ({ page }) => {
  const { email } = await registerCustomer(page);
  await page.getByRole("button", { name: "Open menu", exact: true }).click();
  let drawer = page.getByRole("dialog");
  await expect(drawer.getByText(`Signed in as ${email}`)).toBeVisible();
  await expect(drawer.getByRole("link", { name: /View account/ })).toBeVisible();
  await expect(drawer.getByRole("link", { name: "Sign in", exact: true })).toHaveCount(0);
  await drawer.getByRole("link", { name: /View account/ }).click();
  await page.getByRole("button", { name: "Sign out", exact: true }).last().click();
  await expect(page).toHaveURL(/\/$/);
  await page.getByRole("button", { name: "Open menu", exact: true }).click();
  drawer = page.getByRole("dialog");
  await expect(drawer.getByRole("link", { name: "Sign in", exact: true })).toBeVisible();
  await drawer.getByRole("link", { name: "Sign in", exact: true }).click();
  await main(page).getByLabel("Email").fill(email);
  await main(page).getByLabel("Password", { exact: true }).fill(PASSWORD);
  await main(page).getByRole("button", { name: "Sign in", exact: true }).click();
  await expect(page).toHaveURL(/\/account$/);
  await page.getByRole("button", { name: "Open menu", exact: true }).click();
  drawer = page.getByRole("dialog");
  await expect(drawer.getByText(`Signed in as ${email}`)).toBeVisible();
  await expect(drawer.getByRole("link", { name: "Sign in", exact: true })).toHaveCount(0);
});
