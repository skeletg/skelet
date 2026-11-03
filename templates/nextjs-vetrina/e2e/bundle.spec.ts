import { expect, test, type Page } from "@playwright/test";

import { adminSession, type Admin } from "./admin";
import { main, open } from "./helpers";

type Listing = { channelId: string; isPublished: boolean; effectivePrice: number | null; inStock: boolean };

// A "pick up to 3" box of four listed, in-stock simple products from the dev catalogue
async function seedBundle(admin: Admin): Promise<{ tag: string; bundleId: string }> {
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
  const bundleId = createProduct.id;
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
  return { tag, bundleId };
}

// Seeds the bundle, lets the test adjust it, opens its page and runs the checks - then deletes it again
async function onBundlePage(page: Page, check: () => Promise<void>, adjust?: (admin: Admin, bundleId: string) => Promise<unknown>) {
  const admin = await adminSession();
  let bundleId = "";
  try {
    const seeded = await seedBundle(admin);
    bundleId = seeded.bundleId;
    await adjust?.(admin, bundleId);
    await open(page, `/${seeded.tag}`);
    await check();
  } finally {
    if (bundleId) await admin.gql(`mutation ($id: ID!) { deleteProduct(id: $id) }`, { id: bundleId });
    await admin.dispose();
  }
}

const option = (page: Page) => main(page).getByRole("group", { name: /Fill the box/ });
// The inputs are visually hidden - a shopper clicks the card, here its price
const card = (page: Page, i: number) => option(page).locator("label").nth(i).locator(".tabular-nums");

// A "pick up to 3" option has to stop the 4th tick on the page - the cart refuses it, and a shopper should never get that far
test("a bundle option stops at its maximum pick", async ({ page }) => {
  await onBundlePage(page, async () => {
    await expect(option(page)).toContainText("Pick 1–3");
    const boxes = option(page).getByRole("checkbox");
    for (let i = 0; i < 3; i++) {
      await card(page, i).click();
      await expect(boxes.nth(i)).toBeChecked();
    }
    await expect(option(page)).toContainText("3 picked");
    await expect(boxes.nth(3)).toBeDisabled();
    await card(page, 3).click({ force: true });
    await expect(boxes.nth(3)).not.toBeChecked();

    // Unticking one frees the slot again
    await card(page, 0).click();
    await expect(boxes.nth(3)).toBeEnabled();
  });
});

// A 10%-off bundle says so, and strikes through what the picks add up to
test("a discounted bundle shows its saving against the picks", async ({ page }) => {
  await onBundlePage(
    page,
    async () => {
      await card(page, 0).click();
      await expect(main(page).getByText("Save 10%")).toBeVisible();
      await expect(main(page).locator(".line-through").first()).toBeVisible();
    },
    (admin, bundleId) =>
      admin.gql(`mutation ($input: SetBundlePricingInput!) { setBundlePricing(input: $input) { id } }`, {
        input: { productId: bundleId, channelId: admin.channelId, pricing: "SUM_MINUS_PERCENT", discountPercent: 10 },
      }),
  );
});

// "A hoodie, any size": a configurable selection asks for its variant before the bundle can go in the cart
test("a configurable selection asks for a size, then adds", async ({ page }) => {
  await onBundlePage(
    page,
    async () => {
      const box = option(page);
      const hoodie = box.locator("li").filter({ has: page.getByText(/hoodie/i) }).first();
      await hoodie.locator(".tabular-nums").first().click();
      const size = hoodie.getByRole("combobox");
      await expect(size).toBeVisible();
      await expect(main(page).getByRole("button", { name: "Add bundle to cart" })).toHaveCount(0);
      const firstInStock = await size.locator("option:not([disabled])").first().getAttribute("value");
      await size.selectOption(firstInStock!);
      await main(page).getByRole("button", { name: "Add bundle to cart" }).click();
      await expect(page.getByRole("heading", { name: /Your cart/ })).toBeVisible();
    },
    async (admin, bundleId) => {
      const { products } = await admin.gql<{ products: { id: string; kind: string; children: { id: string }[] }[] }>(
        `query ($ch: ID!) { products(channelId: $ch) { id kind children { id } } }`,
        { ch: admin.channelId },
      );
      const hoodie = products.find((p) => p.kind === "CONFIGURABLE" && p.children.length > 0);
      expect(hoodie, "the dev catalogue needs a configurable with variants").toBeTruthy();
      const { product } = await admin.gql<{ product: { bundleOptions: { id: string }[] } }>(`query ($id: ID!) { product(id: $id) { bundleOptions { id } } }`, { id: bundleId });
      await admin.gql(`mutation ($input: AddBundleSelectionInput!) { addBundleSelection(input: $input) { id } }`, {
        input: { optionId: product.bundleOptions[0].id, productId: hoodie!.id, quantity: 1, sortOrder: 9 },
      });
    },
  );
});

// An add-on deal shows on its pick, and a pick whose amount the customer sets asks how many
test("a selection's deal and customer-set amount show on the page", async ({ page }) => {
  await onBundlePage(
    page,
    async () => {
      await expect(option(page).getByText("−50%")).toBeVisible();
      await card(page, 0).click();
      const howMany = option(page).getByRole("spinbutton", { name: /How many/ });
      await expect(howMany).toHaveValue("1");
      await howMany.fill("3");
      await expect(main(page)).toContainText("3 items per bundle");
    },
    async (admin, bundleId) => {
      const { product } = await admin.gql<{ product: { bundleOptions: { selections: { id: string; sortOrder: number }[] }[] } }>(
        `query ($id: ID!) { product(id: $id) { bundleOptions { selections { id sortOrder } } } }`,
        { id: bundleId },
      );
      const first = product.bundleOptions[0].selections.find((s) => s.sortOrder === 0)!;
      await admin.gql(`mutation ($input: UpdateBundleSelectionInput!) { updateBundleSelection(input: $input) { id } }`, {
        input: { id: first.id, discountPercent: 50, customerQuantity: true },
      });
    },
  );
});
