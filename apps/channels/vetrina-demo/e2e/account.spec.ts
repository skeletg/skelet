import { expect, test } from "@playwright/test";

import { PASSWORD, addSavedAddress, main, open, registerCustomer } from "./helpers";

test.describe("profile", () => {
  test("a cleared name is refused; a saved phone survives a reload", async ({ page }) => {
    await registerCustomer(page);
    await open(page, "/account/profile");
    const form = main(page);

    await form.getByLabel("First name").fill("");
    await form.getByRole("button", { name: "Save changes" }).click();
    await expect(form.getByLabel("First name")).toHaveAccessibleDescription("This field is required.");

    await form.getByLabel("First name").fill("Nina");
    await form.getByLabel("Phone").fill("+381 11 555 0100");
    await form.getByRole("button", { name: "Save changes" }).click();
    await expect(form.getByText("Profile saved.")).toBeVisible();

    await open(page, "/account/profile");
    await expect(main(page).getByLabel("First name")).toHaveValue("Nina");
    await expect(main(page).getByLabel("Phone")).toHaveValue("+381 11 555 0100");
  });
});

test.describe("password", () => {
  test("a wrong current password is refused and every box is cleared; the right one changes it", async ({ page, browser }) => {
    const { email } = await registerCustomer(page);
    await open(page, "/account/password");
    const form = main(page);
    const current = form.getByLabel("Current password");
    const next = form.getByLabel("New password", { exact: true });
    const confirm = form.getByLabel("Confirm new password");

    await current.fill("not-my-password");
    await next.fill("a-new-password-1");
    await confirm.fill("a-new-password-1");
    await form.getByRole("button", { name: "Update password" }).click();
    await expect(form.getByRole("alert")).toBeVisible();
    await expect(current).toHaveValue("");
    await expect(next).toHaveValue("");

    await current.fill(PASSWORD);
    await next.fill("a-new-password-1");
    await confirm.fill("a-new-password-1");
    await form.getByRole("button", { name: "Update password" }).click();
    await expect(form.getByText("Password updated.")).toBeVisible();
    await expect(current).toHaveValue("");

    // The new password is the one that signs in now
    const context = await browser.newContext();
    const fresh = await context.newPage();
    await open(fresh, "/account/login");
    await main(fresh).getByLabel("Email").fill(email);
    await main(fresh).getByLabel("Password", { exact: true }).fill("a-new-password-1");
    await main(fresh).getByRole("button", { name: "Sign in" }).click();
    await expect(fresh).toHaveURL(/\/account$/);
    await context.close();
  });
});

test.describe("addresses", () => {
  test("a missing field is caught before posting; a good address is saved, listed and editable", async ({ page }) => {
    await registerCustomer(page);
    await open(page, "/account/addresses/new");
    const form = main(page);
    await form.getByRole("button", { name: "Save address" }).click();
    await expect(form.getByLabel("Street address")).toHaveAccessibleDescription(/This field is required/);

    await addSavedAddress(page, { street1: "350 Fifth Avenue", city: "New York", state: "NY", postalCode: "10118" });
    const card = main(page).locator("address");
    await expect(card).toContainText("350 Fifth Avenue");
    await expect(card).toContainText("New York, NY 10118");
    await expect(card).toContainText("US");

    await main(page).getByRole("link", { name: "Edit" }).click();
    await expect(page).toHaveURL(/\/account\/addresses\/.+\/edit$/);
    await open(page, page.url());
    await main(page).getByLabel("City").fill("Brooklyn");
    await main(page).getByRole("button", { name: "Save changes" }).click();
    await expect(page).toHaveURL(/\/account\/addresses$/);
    await expect(main(page).locator("address")).toContainText("Brooklyn");
  });
});
