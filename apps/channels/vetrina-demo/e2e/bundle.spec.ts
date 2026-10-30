import { expect, test } from "@playwright/test";

import { adminSession } from "./admin";
import { main, open } from "./helpers";

type Listing = { channelId: string; isPublished: boolean; effectivePrice: number | null; inStock: boolean };

// A "pick up to 3" option has to stop the 4th tick on the page - the cart refuses it, and a shopper should never get that far
test("a bundle option stops at its maximum pick", async ({ page }) => {
  const admin = await adminSession();
  let bundleId = "";
  try {
    const ch = admin.channelId;
    const { channel, productTypes, products } = await admin.gql<{
      channel: { defaultLocale: string; currencyCode: string };
      productTypes: { id: string }[];
      products: { id: string; kind: string; name: string | null; listings: Listing[] }[];
    }>(
      `query ($ch: ID!) {
        channel(id: $ch) { defaultLocale currencyCode }
        productTypes { id }
        products(channelId: $ch) { id kind name listings { channelId isPublished effectivePrice inStock } }
      }`,
      { ch },
    );
    const parts = products
      .filter((p) => p.kind === "SIMPLE" && p.name && p.listings.some((l) => l.channelId === ch && l.isPublished && l.inStock && l.effectivePrice != null))
      .slice(0, 4);
    expect(parts, "the dev catalogue needs four listed, in-stock simple products").toHaveLength(4);

    const tag = `e2e-bundle-${Date.now()}`;
    const locale = channel.defaultLocale;
    const { createProduct } = await admin.gql<{ createProduct: { id: string } }>(
      `mutation ($input: CreateProductInput!) { createProduct(input: $input) { id } }`,
      { input: { productTypeId: productTypes[0].id, kind: "BUNDLE", sku: tag, url: tag, locale, name: `Pick box ${tag}`, defaultPrice: 0, defaultCurrencyCode: channel.currencyCode } },
    );
    bundleId = createProduct.id;
    await admin.gql(`mutation ($input: SetProductListingInput!) { setProductListing(input: $input) { id } }`, {
      input: { productId: bundleId, channelId: ch, isPublished: true },
    });
    const { addBundleOption } = await admin.gql<{ addBundleOption: { bundleOptions: { id: string }[] } }>(
      `mutation ($input: AddBundleOptionInput!) { addBundleOption(input: $input) { bundleOptions { id } } }`,
      { input: { productId: bundleId, inputType: "CHECKBOX", isRequired: true, maxSelections: 3, locale, title: "Fill the box" } },
    );
    const optionId = addBundleOption.bundleOptions[0].id;
    for (const [i, p] of parts.entries()) {
      await admin.gql(`mutation ($input: AddBundleSelectionInput!) { addBundleSelection(input: $input) { id } }`, {
        input: { optionId, productId: p.id, quantity: 1, sortOrder: i },
      });
    }

    await open(page, `/${tag}`);
    const option = main(page).getByRole("group", { name: /Fill the box/ });
    await expect(option).toContainText("Pick 1–3");
    const boxes = option.getByRole("checkbox");
    // The inputs are visually hidden - a shopper clicks the card, here its price
    const card = (i: number) => option.locator("label").nth(i).locator(".tabular-nums");
    for (let i = 0; i < 3; i++) {
      await card(i).click();
      await expect(boxes.nth(i)).toBeChecked();
    }
    await expect(option).toContainText("3 picked");
    await expect(boxes.nth(3)).toBeDisabled();
    await expect(boxes.nth(3)).not.toBeChecked();

    // Unticking one frees the slot again
    await card(3).click({ force: true });
    await expect(boxes.nth(3)).not.toBeChecked();
    await card(0).click();
    await expect(boxes.nth(3)).toBeEnabled();
  } finally {
    if (bundleId) await admin.gql(`mutation ($id: ID!) { deleteProduct(id: $id) }`, { id: bundleId });
    await admin.dispose();
  }
});
