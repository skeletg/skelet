import { expect, type Page } from "@playwright/test";

// Every spec signs up its own throwaway customer, so specs never share an account and a run leaves only
// e2e-<timestamp>@skelet.test rows behind in the dev database
export function uniqueEmail(tag: string): string {
  return `e2e-${tag}-${Date.now()}-${Math.random().toString(36).slice(2, 7)}@skelet.test`;
}

export const PASSWORD = "e2e-password-1";

// The page's own form - the header also mounts a sign-in drawer with the same fields
export function main(page: Page) {
  return page.getByRole("main");
}

// Waits for hydration before the first interaction: a click that lands before React attaches submits the form
// natively, which still works but is not the path a shopper takes
export async function open(page: Page, path: string) {
  await page.goto(path);
  await page.waitForLoadState("networkidle");
}

// Fills the account's new-address form and saves it, landing back on the address book. US, because it's in the dev
// reference data with its states seeded
export async function addSavedAddress(
  page: Page,
  a: { street1: string; city: string; state: string; postalCode: string },
) {
  await open(page, "/account/addresses/new");
  const form = main(page);
  await form.getByLabel("First name").fill("Ana");
  await form.getByLabel("Last name").fill("E2E");
  await form.getByLabel("Street address").fill(a.street1);
  await form.getByLabel("City").fill(a.city);
  await form.getByLabel("Postal code").fill(a.postalCode);
  await form.getByLabel("Country").selectOption("US");
  await form.getByLabel("State / region").selectOption(a.state);
  await form.getByRole("button", { name: "Save address" }).click();
  await expect(page).toHaveURL(/\/account\/addresses$/);
}

export async function registerCustomer(
  page: Page,
  { firstName = "Ana", lastName = "E2E", email = uniqueEmail("customer") } = {},
): Promise<{ email: string; password: string }> {
  await open(page, "/account/register");
  const form = main(page);
  await form.getByLabel("First name").fill(firstName);
  await form.getByLabel("Last name").fill(lastName);
  await form.getByLabel("Email", { exact: true }).fill(email);
  await form.getByLabel("Password", { exact: true }).fill(PASSWORD);
  await form.getByLabel("Confirm password").fill(PASSWORD);
  await form.getByRole("button", { name: "Create account" }).click();
  // ?welcome=1 is stripped client-side once the sign-up analytics event fires, so match the page, not the query
  await expect(page).toHaveURL(/\/account(\?|$)/);
  return { email, password: PASSWORD };
}
