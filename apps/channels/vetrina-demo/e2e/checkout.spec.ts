import { expect, test, type Page } from "@playwright/test";

import { adminSession, type Admin } from "./admin";
import { addSavedAddress, main, open, registerCustomer, uniqueEmail } from "./helpers";

// The dev catalogue's one simple, in-stock product
const PRODUCT = "/casual-t-shirt";

async function addToCart(page: Page) {
  await open(page, PRODUCT);
  await main(page).getByRole("button", { name: "Add to cart" }).first().click();
  await expect(page.getByRole("button", { name: /Open cart \([1-9]/ })).toBeVisible();
}

// Step 1 on the dev store's US rate - a postcode alone brings up the options
async function chooseDelivery(page: Page, postalCode: string) {
  const form = main(page);
  const country = form.getByLabel("Country");
  if (await country.isVisible()) await country.selectOption("US");
  await form.getByLabel("Postcode").fill(postalCode);
  await form.getByRole("button", { name: "Show options" }).click();
  await expect(form.getByRole("radio", { name: /Standard shipping/ })).toBeChecked();
  await form.getByRole("button", { name: "Continue to details" }).click();
}

// Clicks an address card's make-default button unless the card already carries that default. The button goes as soon
// as it's pending ("Saving…"), so the wait is on the tag that replaces it - leaving earlier aborts the save
async function makeDefault(page: Page, street1: string, which: "Default shipping" | "Default billing") {
  const card = main(page).getByRole("listitem").filter({ hasText: street1 });
  const button = card.getByRole("button", { name: which });
  if (await button.isVisible()) await button.click();
  await expect(button).toHaveCount(0);
  await expect(card.getByText(which, { exact: true })).toBeVisible();
}

// Each step saves to the cart, so the checkout survives a reload. Placement itself is covered by the backend's
// money-path suite - this walks the three steps up to payment on the dev store's US rate
test.describe("checkout", () => {
  test("a guest goes from a postcode to payment, and every step survives a reload", async ({ page }) => {
    await addToCart(page);
    await open(page, "/checkout");
    const form = main(page);

    // Step 1 - a postcode alone brings up the options
    const postcode = form.getByLabel("Postcode");
    await form.getByRole("button", { name: "Show options" }).click();
    await expect(postcode).toHaveAccessibleDescription(/Enter a postcode/);
    const country = form.getByLabel("Country");
    if (await country.isVisible()) await country.selectOption("US");
    await postcode.fill("12345");
    await form.getByRole("button", { name: "Show options" }).click();
    await expect(form.getByRole("radio", { name: /Standard shipping/ })).toBeChecked();
    await form.getByRole("button", { name: "Continue to details" }).click();

    // Step 2 - the postcode carries over into the address
    const email = form.getByLabel("Email", { exact: true });
    await expect(email).toBeVisible();
    await expect(form.getByLabel("Postal code")).toHaveValue("12345");
    await form.getByRole("button", { name: "Continue to payment" }).click();
    await expect(email).toHaveAccessibleDescription(/This field is required/);

    const address = uniqueEmail("checkout");
    await email.fill(address);
    await form.getByLabel("First name").fill("Ana");
    await form.getByLabel("Last name").fill("E2E");
    await form.getByLabel("Street address").fill("1 Test Way");
    await form.getByLabel("City").fill("Testville");
    await form.getByRole("button", { name: "Continue to payment" }).click();

    // Step 3 opens with both earlier steps folded to their summaries
    await expect(form.getByRole("button", { name: /^(Pay|Place order)/ })).toBeVisible();
    await expect(form.getByText(address)).toBeVisible();
    await expect(form.getByText(/1 Test Way, Testville 12345, US/)).toBeVisible();

    // A reload re-reads the cart - nothing chosen so far is lost
    await page.reload();
    await page.waitForLoadState("networkidle");
    await expect(form.getByText(/Standard shipping/)).toBeVisible();
    await expect(form.getByText(address)).toBeVisible();

    // Edit reopens the details with what the cart holds, and saving returns to payment
    await form.getByRole("link", { name: "Edit" }).nth(1).click();
    await expect(form.getByLabel("City")).toHaveValue("Testville");
    await form.getByLabel("City").fill("Newtown");
    await form.getByRole("button", { name: "Continue to payment" }).click();
    await expect(form.getByText(/1 Test Way, Newtown 12345, US/)).toBeVisible();
    await expect(page).not.toHaveURL(/step=/);
  });

  test("a signed-in customer's default addresses are preselected, and a new one can be saved to the book", async ({
    page,
  }) => {
    await registerCustomer(page);
    const home = { street1: "1 Home Street", city: "New York", state: "NY", postalCode: "10118" };
    const office = { street1: "2 Office Avenue", city: "New York", state: "NY", postalCode: "10118" };
    await addSavedAddress(page, home);
    await addSavedAddress(page, office);
    await makeDefault(page, home.street1, "Default shipping");
    await makeDefault(page, office.street1, "Default billing");

    await addToCart(page);
    await open(page, "/checkout");
    await chooseDelivery(page, "10118");

    // Step 2 - no email to type, the defaults are picked, and billing opens on its own default
    const form = main(page);
    const delivery = form.getByRole("region", { name: "Delivery address" });
    const billing = form.getByRole("region", { name: "Billing address" });
    await expect(form.getByLabel("Email", { exact: true })).toHaveCount(0);
    await expect(delivery.getByRole("radio", { name: /1 Home Street/ })).toBeChecked();
    await expect(delivery.getByLabel("Street address")).toHaveCount(0);
    await expect(billing.getByRole("checkbox", { name: "Same as the delivery address" })).not.toBeChecked();
    await expect(billing.getByRole("radio", { name: /2 Office Avenue/ })).toBeChecked();
    await form.getByRole("button", { name: "Continue to payment" }).click();

    await expect(form.getByRole("button", { name: /^(Pay|Place order)/ })).toBeVisible();
    await expect(form.getByText(/1 Home Street, New York 10118, US/)).toBeVisible();

    // A new delivery address with "save" ticked lands in the book beside the other two
    await form.getByRole("link", { name: "Edit" }).nth(1).click();
    await delivery.getByRole("radio", { name: "Use a new address" }).click();
    await delivery.getByLabel("First name").fill("Ana");
    await delivery.getByLabel("Last name").fill("E2E");
    await delivery.getByLabel("Street address").fill("3 New Road");
    await delivery.getByLabel("City").fill("New York");
    await delivery.getByLabel("State / region").selectOption("NY");
    await delivery.getByLabel("Save to my addresses").check();
    await form.getByRole("button", { name: "Continue to payment" }).click();
    await expect(form.getByText(/3 New Road, New York 10118, US/)).toBeVisible();

    await open(page, "/account/addresses");
    await expect(main(page).locator("address")).toHaveCount(3);
    await expect(main(page).locator("address").filter({ hasText: "3 New Road" })).toBeVisible();
  });
});

// Several ways to ship to one address: the store's US zone gains Express (next day, weekends too), Economy (no estimate)
// and a free method beside the dev store's Standard shipping. Seeded through the admin API after the tests above, which
// expect Standard shipping to be the only choice - one file's tests run in order on one worker - and removed after
test.describe("checkout delivery choices", () => {
  const PREFIX = "e2e-delivery-";
  let admin: Admin;

  const ZONES = `query ($channelId: ID!) { shippingZones(channelId: $channelId) { id countries methods { id code } } }`;
  async function usZone() {
    const { shippingZones } = await admin.gql<{ shippingZones: { id: string; countries: string[]; methods: { id: string; code: string }[] }[] }>(ZONES, {
      channelId: admin.channelId,
    });
    const zone = shippingZones.find((z) => z.countries.includes("US"));
    if (!zone) throw new Error("the dev store has no US shipping zone");
    return zone;
  }
  async function removeSeeded() {
    for (const m of (await usZone()).methods.filter((m) => m.code.startsWith(PREFIX))) {
      await admin.gql("mutation ($id: ID!) { deleteShippingMethod(id: $id) }", { id: m.id });
    }
  }

  test.beforeAll(async () => {
    admin = await adminSession();
    // A run cut short leaves its methods behind, and they'd sit beside Standard shipping for the tests above next time
    await removeSeeded();
    const zone = await usZone();
    const suffix = Date.now().toString(36);
    for (const m of [
      { code: "express", name: "Express", calcType: "FLAT", flatCents: 1200, estimatedDays: 1, deliversOnWeekends: true, sortOrder: 1 },
      { code: "economy", name: "Economy", calcType: "FLAT", flatCents: 300, estimatedDays: 0, deliversOnWeekends: false, sortOrder: 2 },
      { code: "free", name: "Free delivery", calcType: "FREE", flatCents: 0, estimatedDays: 6, deliversOnWeekends: false, sortOrder: 3 },
    ]) {
      await admin.gql(
        "mutation ($input: CreateShippingMethodInput!) { createShippingMethod(input: $input) { id } }",
        { input: { ...m, code: `${PREFIX}${m.code}-${suffix}`, zoneId: zone.id, isActive: true, deliveryKind: "ship_to_address" } },
      );
    }
  });

  test.afterAll(async () => {
    if (!admin) return;
    await removeSeeded();
    await admin.dispose();
  });

  // The date the store's calendar gives a quote today - the dev store keeps UTC, and the storefront labels in "en"
  function arrives(days: number, weekends: boolean) {
    const d = new Date();
    for (let left = days; left > 0; ) {
      d.setUTCDate(d.getUTCDate() + 1);
      const wd = d.getUTCDay();
      if (weekends || (wd !== 0 && wd !== 6)) left--;
    }
    return new Intl.DateTimeFormat("en", { weekday: "short", day: "numeric", month: "short", timeZone: "UTC" }).format(d);
  }

  test("a shopper sees every method with its price and arrival, and the choice survives a reload", async ({ page }) => {
    await addToCart(page);
    await open(page, "/checkout");
    const form = main(page);
    const country = form.getByLabel("Country");
    if (await country.isVisible()) await country.selectOption("US");
    await form.getByLabel("Postcode").fill("12345");
    await form.getByRole("button", { name: "Show options" }).click();

    const card = (name: RegExp) => form.getByRole("radio", { name }).locator("xpath=ancestor::label");
    // Under the dev store's one door option, so each card names its method - beside whatever pickup the store offers
    for (const name of [/Delivery\s*· Standard shipping/, /Delivery\s*· Express/, /Delivery\s*· Economy/, /Delivery\s*· Free delivery/]) {
      await expect(form.getByRole("radio", { name })).toBeVisible();
    }
    await expect(card(/Express/)).toContainText("12.00");
    await expect(card(/Express/)).toContainText(`Arrives ${arrives(1, true)}`);
    await expect(card(/Express/)).toContainText("Fastest");
    await expect(card(/Free delivery/)).toContainText("Free");
    await expect(card(/Free delivery/)).toContainText(`Arrives ${arrives(6, false)}`);
    await expect(card(/Economy/)).toContainText("3.00");
    await expect(card(/Economy/)).not.toContainText("Arrives");

    await form.getByRole("radio", { name: /Express/ }).check();
    await expect(form.getByRole("radio", { name: /Express/ })).toBeChecked();
    await page.reload();
    await page.waitForLoadState("networkidle");
    await expect(form.getByRole("radio", { name: /Express/ })).toBeChecked();

    // With every group chosen, Continue is a link to the next step, and the delivery step folds to what was picked
    await form.getByRole("link", { name: "Continue to details" }).click();
    await expect(form.getByLabel("Email", { exact: true })).toBeVisible();
    await expect(form.getByText(/Delivery · Express · \D*12\.00/)).toBeVisible();
  });
});
