import { expect, test, type Page } from "@playwright/test";

import { main, open, registerCustomer } from "./helpers";

// The dev catalogue's one simple, in-stock product
const PRODUCT = "/casual-t-shirt";

async function addToCart(page: Page) {
  await open(page, PRODUCT);
  await main(page).getByRole("button", { name: "Add to cart" }).first().click();
  await expect(page.getByRole("button", { name: /Open cart \([1-9]/ })).toBeVisible();
}

test.describe("cart", () => {
  test("add to cart opens the drawer and counts the item", async ({ page }) => {
    await addToCart(page);
    await expect(page.getByRole("heading", { name: /Your cart/ })).toBeVisible();
  });

  test("the coupon form asks for a code, and names an unknown one under the input it came from", async ({ page }) => {
    await addToCart(page);
    await open(page, "/cart");
    const code = main(page).getByLabel("Promo code");
    const apply = main(page).getByRole("button", { name: "Apply" });

    await apply.click();
    await expect(code).toHaveAccessibleDescription("Enter a code.");

    await code.fill("NO-SUCH-CODE-E2E");
    await apply.click();
    await expect(code).toHaveAccessibleDescription("That code isn't valid.");
    await expect(code).toHaveValue("NO-SUCH-CODE-E2E");
  });
});

test.describe("questions", () => {
  test("a guest question is checked for length and a display name, then thanked", async ({ page }) => {
    await open(page, PRODUCT);
    await main(page).getByRole("button", { name: "Ask a question" }).click();
    const question = main(page).getByLabel("Your question");
    const submit = main(page).getByRole("button", { name: "Submit question" });

    await question.fill("Too short");
    await submit.click();
    await expect(question).toHaveAccessibleDescription("Questions need at least 10 characters.");
    await expect(main(page).getByLabel("Display name")).toHaveAccessibleDescription("This field is required.");

    await question.fill("Does this shirt shrink after the first wash?");
    await main(page).getByLabel("Display name").fill("E2E shopper");
    await submit.click();
    await expect(main(page).getByText(/your question was sent to the store team/)).toBeVisible();
  });
});

test.describe("reviews", () => {
  // Regression: the review page read eligibility through a query sent without the session, so every signed-in
  // customer bounced between it and the login page until the browser gave up
  test("a signed-in customer who hasn't bought the product is sent back to it, not into a login loop", async ({ page }) => {
    // The product id comes from the guest's "Sign in to write a review" link - signed in, the page says why the
    // customer can't review yet instead of linking
    await open(page, PRODUCT);
    const href = await main(page).locator("a[href*='review']").first().getAttribute("href");
    const id = decodeURIComponent(href ?? "").match(/products\/([^/]+)\/review/)?.[1];
    expect(id).toBeTruthy();
    await registerCustomer(page);

    const response = await page.goto(`/products/${id}/review`);
    expect(response?.ok()).toBe(true);
    await expect(page).toHaveURL(new RegExp(`${PRODUCT}$`));
  });
});
