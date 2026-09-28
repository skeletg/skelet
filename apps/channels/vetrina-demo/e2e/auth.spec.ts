import { expect, test } from "@playwright/test";

import { PASSWORD, main, open, registerCustomer, uniqueEmail } from "./helpers";

test.describe("register", () => {
  test("a blank submit is stopped in the browser with an error on each required field", async ({ page }) => {
    await open(page, "/account/register");
    const form = main(page);
    await form.getByRole("button", { name: "Create account" }).click();

    await expect(form.getByLabel("First name")).toHaveAccessibleDescription("This field is required.");
    await expect(form.getByLabel("Email", { exact: true })).toHaveAccessibleDescription("This field is required.");
    await expect(page).toHaveURL(/\/account\/register$/);
  });

  test("mismatched passwords are flagged on the confirm field and nothing else is lost", async ({ page }) => {
    await open(page, "/account/register");
    const form = main(page);
    await form.getByLabel("First name").fill("Ana");
    await form.getByLabel("Last name").fill("E2E");
    await form.getByLabel("Email", { exact: true }).fill(uniqueEmail("mismatch"));
    await form.getByLabel("Password", { exact: true }).fill(PASSWORD);
    await form.getByLabel("Confirm password").fill("something-else");
    await form.getByRole("button", { name: "Create account" }).click();

    await expect(form.getByLabel("Confirm password")).toHaveAccessibleDescription("Passwords do not match.");
    await expect(form.getByLabel("First name")).toHaveValue("Ana");
  });

  test("a valid sign-up lands on the account overview, signed in", async ({ page }) => {
    await registerCustomer(page, { firstName: "Mila" });
    await expect(main(page).getByRole("heading", { name: "Hi, Mila" })).toBeVisible();
  });
});

test.describe("sign in", () => {
  test("a wrong password keeps the email, clears the password and says why", async ({ browser }) => {
    // The account is made in its own browser, so this one starts signed out
    const setup = await browser.newContext();
    const { email } = await registerCustomer(await setup.newPage());
    await setup.close();

    const context = await browser.newContext();
    const page = await context.newPage();
    await open(page, "/account/login");
    const form = main(page);
    await form.getByLabel("Email").fill(email);
    await form.getByLabel("Password").fill("not-the-password");
    await form.getByRole("button", { name: "Sign in" }).click();

    await expect(form.getByRole("alert")).toBeVisible();
    await expect(form.getByLabel("Email")).toHaveValue(email);
    await expect(form.getByLabel("Password")).toHaveValue("");
    await expect(page).toHaveURL(/\/account\/login/);
    await context.close();
  });

  test("the right password goes back to where the shopper came from", async ({ browser }) => {
    const setup = await browser.newContext();
    const { email } = await registerCustomer(await setup.newPage());
    await setup.close();

    const context = await browser.newContext();
    const page = await context.newPage();
    await open(page, "/account/login?returnTo=%2Fcart");
    const form = main(page);
    await form.getByLabel("Email").fill(email);
    await form.getByLabel("Password").fill(PASSWORD);
    await form.getByRole("button", { name: "Sign in" }).click();

    await expect(page).toHaveURL(/\/cart$/);
    await context.close();
  });

  // fixme: the forms post and validate without JavaScript, but the route's loading.tsx means the page itself only
  // arrives as a streamed Suspense swap - and that swap is an inline script. With JS off the shopper sees the
  // skeleton forever. Unblock by dropping loading.tsx above /account, or accept that the storefront needs JS
  test.fixme("without JavaScript the server still validates and answers with field errors", async ({ browser }) => {
    const context = await browser.newContext({ javaScriptEnabled: false });
    const page = await context.newPage();
    await page.goto("/account/login");
    const form = main(page);
    await form.getByLabel("Email").fill("not-an-email");
    await form.getByRole("button", { name: "Sign in" }).click();

    await expect(form.getByText("Enter a valid email address.")).toBeVisible();
    await expect(form.getByText("This field is required.")).toBeVisible();
    await context.close();
  });
});

test.describe("forgot password", () => {
  test("a malformed email is flagged; a well-formed one gets the same answer whether or not it exists", async ({ page }) => {
    await open(page, "/account/forgot");
    const form = main(page);
    await form.getByLabel("Email").fill("nope");
    await form.getByRole("button", { name: "Send reset link" }).click();
    await expect(form.getByLabel("Email")).toHaveAccessibleDescription("Enter a valid email address.");

    await form.getByLabel("Email").fill(uniqueEmail("nobody"));
    await form.getByRole("button", { name: "Send reset link" }).click();
    await expect(main(page).getByText(/If an account exists for that email/)).toBeVisible();
  });
});
