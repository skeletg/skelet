import { expect, test } from "@playwright/test";

import { main } from "./helpers";

// Catalog content is in the first HTML, visible with scripting off - what a non-JS crawler or a shopper on a script
// that failed to load sees. A route-level loading.tsx breaks this: the page streams in a hidden segment that only
// React's inline script swaps in over the skeleton
test.use({ javaScriptEnabled: false });

// The dev catalogue's one simple, in-stock product
const PRODUCT = "/casual-t-shirt";

test("a product page shows its name, price, stock and SKU", async ({ page }) => {
  await page.goto(PRODUCT);
  const pdp = main(page);
  await expect(pdp.getByRole("heading", { level: 1, name: "Casual T-Shirt" })).toBeVisible();
  await expect(pdp.getByText("$15.00").first()).toBeVisible();
  await expect(pdp.getByText("In stock").first()).toBeVisible();
  await expect(pdp.getByText("SKU casual-t-shirt")).toBeVisible();
});

// The dev homepage is CMS-built, so this also renders every block type on it in one pass
test("the home page renders in full", async ({ page }) => {
  const res = await page.goto("/");
  expect(res?.status()).toBe(200);
  await expect(main(page).getByRole("heading", { level: 1 })).toBeVisible();
});

for (const path of ["/products", "/search?q=shirt"]) {
  test(`${path} lists its products, with no skeleton standing in`, async ({ page }) => {
    await page.goto(path);
    const listing = main(page);
    await expect(listing.getByRole("heading", { level: 1 })).toBeVisible();
    await expect(listing.getByRole("link", { name: /Casual T-Shirt/ }).first()).toBeVisible();
    await expect(listing.getByText("$15.00").first()).toBeVisible();
    await expect(page.getByRole("status", { name: /Loading/ })).toHaveCount(0);
  });
}
