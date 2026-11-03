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

// Analytics on a shared computer: whoever browses after a sign-out is a new visitor, not the account that left
test("signing out starts a new visitor id and analytics session", async ({ page, context, baseURL }) => {
  await context.addCookies([{ name: "storefront_consent", value: "v2:a1.d0.p0", url: baseURL! }]);
  await registerCustomer(page);
  const ids = async () => {
    const jar = await context.cookies();
    return { aid: jar.find((c) => c.name === "lens_aid")?.value, sid: jar.find((c) => c.name === "sk_sid")?.value };
  };
  const before = await ids();
  expect(before.aid).toBeTruthy();
  await page.goto("/account");
  await page.getByRole("button", { name: "Sign out", exact: true }).last().click();
  await expect(page).toHaveURL(/\/$/);
  await page.reload();
  const after = await ids();
  expect(after.aid).toBeTruthy();
  expect(after.aid).not.toBe(before.aid);
  expect(after.sid).toBeTruthy();
  expect((await context.cookies()).some((c) => c.name === "sk_signed_out")).toBe(false);
});
