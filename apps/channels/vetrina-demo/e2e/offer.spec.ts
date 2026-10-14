import { expect, test } from "@playwright/test";

import { adminSession, type Admin } from "./admin";
import { main, open, PASSWORD, registerCustomer } from "./helpers";

// Cart offers: the store builds a cart for one recipient and sends it as a link. These walk the link the way a
// recipient does - the backend's money-path suite covers placement and every lock

const CART = `id offer { linkPath } lines { id }`;

async function guestOffer(admin: Admin, email: string): Promise<{ id: string; path: string }> {
  const { adminCreateCartOffer: cart } = await admin.gql<{ adminCreateCartOffer: { id: string } }>(
    `mutation ($input: AdminCreateCartOfferInput!) { adminCreateCartOffer(input: $input) { ${CART} } }`,
    { input: { channelId: admin.channelId, email } },
  );
  return finish(admin, cart.id);
}

// A custom line, a message and the lines locked - then the link's path, which the storefront under test serves whatever
// public URL the channel has
async function finish(admin: Admin, id: string): Promise<{ id: string; path: string }> {
  await admin.gql(`mutation ($cartId: ID!, $input: AdminCustomCartLineInput!) { adminAddCustomCartLine(cartId: $cartId, input: $input) { id } }`, {
    cartId: id,
    input: { name: "Installation", unitPrice: 2500, quantity: 1, taxClass: "standard" },
  });
  const { adminUpdateCartOffer: cart } = await admin.gql<{ adminUpdateCartOffer: { offer: { linkPath: string } } }>(
    `mutation ($cartId: ID!, $input: AdminUpdateCartOfferInput!) { adminUpdateCartOffer(cartId: $cartId, input: $input) { ${CART} } }`,
    {
      cartId: id,
      input: {
        expiresAt: new Date(Date.now() + 86_400_000).toISOString(),
        message: "As discussed on the phone.",
        showInAccount: false,
        locks: ["LINES"],
        shippingAddress: null,
        billingSameAsShipping: false,
        billingAddress: null,
        paymentMethods: [],
        deliveries: [],
      },
    },
  );
  expect(cart.offer.linkPath).toMatch(/^\/offer\/[A-Za-z0-9_-]{22}$/);
  return { id, path: cart.offer.linkPath };
}

test.describe("cart offers", () => {
  let admin: Admin;
  test.beforeAll(async () => {
    admin = await adminSession();
  });
  test.afterAll(async () => {
    await admin?.dispose();
  });

  test("a guest opens the link, sees the locked offer, and checks it out beside an empty own cart", async ({ page }) => {
    const offer = await guestOffer(admin, `e2e-offer-${Date.now()}@skelet.test`);

    // The id alone is no way in, and a tampered link explains itself
    await open(page, offer.path.slice(0, -2) + "xx");
    await expect(main(page).getByRole("heading", { name: "This link isn’t valid" })).toBeVisible();

    await open(page, offer.path);
    await expect(page).toHaveURL(/\/cart\/offer$/);
    const form = main(page);
    await expect(form.getByRole("heading", { name: "Your offer" })).toBeVisible();
    await expect(form.getByText("Installation")).toBeVisible();
    await expect(form.getByText("As discussed on the phone.")).toBeVisible();
    await expect(form.getByText(/Set by the store: the items/)).toBeVisible();
    await expect(form.getByRole("button", { name: "Update" })).toHaveCount(0);

    await form.getByRole("button", { name: /Continue to checkout/ }).click();
    await expect(page).toHaveURL(/\/checkout/);
    await expect(main(page).getByText("You’re checking out an offer the store prepared for you.")).toBeVisible();
    await expect(page.getByRole("complementary").getByText("Installation")).toBeVisible();

    // Back to the shopper's own cart - the offer never became it
    await main(page).getByRole("link", { name: "Back to your own cart" }).click();
    await expect(page).toHaveURL(/\/cart$/);
    await expect(main(page).getByText("Your cart is empty.")).toBeVisible();
  });

  test("an offer made out to an account asks its owner to sign in, then opens", async ({ page, browser }) => {
    const { email } = await registerCustomer(page);
    const { customersPaged } = await admin.gql<{ customersPaged: { items: { id: string }[] } }>(
      `query ($channelId: ID, $search: String) { customersPaged(channelId: $channelId, search: $search, limit: 1) { items { id } } }`,
      { channelId: admin.channelId, search: email },
    );
    const customerId = customersPaged.items[0]?.id;
    if (!customerId) throw new Error(`customer ${email} not found`);
    const { adminCreateCartOffer: cart } = await admin.gql<{ adminCreateCartOffer: { id: string } }>(
      `mutation ($input: AdminCreateCartOfferInput!) { adminCreateCartOffer(input: $input) { ${CART} } }`,
      { input: { channelId: admin.channelId, customerId } },
    );
    const offer = await finish(admin, cart.id);

    const context = await browser.newContext();
    const fresh = await context.newPage();
    await open(fresh, offer.path);
    await expect(fresh).toHaveURL(/\/account\/login\?returnTo=/);
    await main(fresh).getByLabel("Email").fill(email);
    await main(fresh).getByLabel("Password", { exact: true }).fill(PASSWORD);
    await main(fresh).getByRole("button", { name: "Sign in" }).click();
    await expect(fresh).toHaveURL(/\/cart\/offer$/);
    await expect(main(fresh).getByText("Installation")).toBeVisible();
    await context.close();
  });

  test("account offers keep their total and clear the new indicator after opening", async ({ page }) => {
    const { email } = await registerCustomer(page);
    const { customersPaged } = await admin.gql<{ customersPaged: { items: { id: string }[] } }>(
      `query ($channelId: ID, $search: String) { customersPaged(channelId: $channelId, search: $search, limit: 1) { items { id } } }`,
      { channelId: admin.channelId, search: email },
    );
    const { adminCreateCartOffer: cart } = await admin.gql<{ adminCreateCartOffer: { id: string } }>(
      `mutation ($input: AdminCreateCartOfferInput!) { adminCreateCartOffer(input: $input) { ${CART} } }`,
      { input: { channelId: admin.channelId, customerId: customersPaged.items[0].id } },
    );
    await finish(admin, cart.id);
    await admin.gql(`mutation ($cartId: ID!, $input: AdminUpdateCartOfferInput!) { adminUpdateCartOffer(cartId: $cartId, input: $input) { id } }`, {
      cartId: cart.id,
      input: { expiresAt: new Date(Date.now() + 86_400_000).toISOString(), message: "As discussed on the phone.", showInAccount: true, locks: [], billingSameAsShipping: false, paymentMethods: [], deliveries: [] },
    });
    await open(page, "/account/offers");
    await expect(page.getByLabel("1 offer, 1 new offer").last()).toBeVisible();
    await expect(main(page).getByText("New", { exact: true })).toBeVisible();
    await main(page).getByRole("button", { name: "Open", exact: true }).click();
    await expect(page).toHaveURL(/\/cart\/offer$/);
    // Equal-height actions keep their text centered on the same row.
    const update = await main(page).getByRole("button", { name: "Update", exact: true }).boundingBox();
    const remove = await main(page).getByRole("button", { name: "Remove", exact: true }).boundingBox();
    expect(update).not.toBeNull();
    expect(remove).not.toBeNull();
    expect(Math.abs(update!.y - remove!.y)).toBeLessThan(1);
    expect(update!.height).toBe(remove!.height);
    await open(page, "/account/offers");
    await expect(page.getByLabel("1 offer", { exact: true }).last()).toBeVisible();
    await expect(page.getByLabel("1 offer, 1 new offer")).toHaveCount(0);
    await expect(main(page).getByText("New", { exact: true })).toHaveCount(0);
  });
});
