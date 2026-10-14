import { expect, test } from "@playwright/test";

// Read-only journeys over the explicitly installed dev-store-demo fixtures.
// No orders, cart writes, stock holds or inventory adjustments are made here.
test.describe("store availability", () => {
  test.beforeEach(async ({ request }) => {
    const response = await request.get("/api/stores");
    expect(response.ok(), "Store API should be reachable").toBeTruthy();
    const gate = await response.json();
    test.skip(!gate.enabled, "Install dev-store-demo fixtures to enable these journeys");
  });
  test("checks a product across cities and remembers a preferred store", async ({ page }) => {
    await page.goto("/store-demo-backpack");
    const check = page.getByRole("button", { name: "Check availability in stores" });
    await check.click();
    const dialog = page.getByRole("dialog", { name: "Check availability in stores" });
    await expect(dialog).toBeVisible();
    await expect(dialog.getByText("Demo · Belgrade Central")).toBeVisible();
    await expect(dialog.getByText("In-store shopping", { exact: true }).first()).toBeVisible();
    await expect(dialog.getByText("Demo · Private warehouse")).toHaveCount(0);
    await expect(dialog.getByText("Availability unknown", { exact: true })).toBeVisible();
    await dialog.getByRole("combobox", { name: "City", exact: true }).selectOption("Belgrade");
    await expect(dialog.getByText("Demo · Novi Sad")).toHaveCount(0);
    await dialog.getByRole("button", { name: "Set as my store" }).click();
    await expect(dialog.getByRole("button", { name: "My store", exact: true })).toBeDisabled();
    await page.keyboard.press("Escape");
    await expect(dialog).toHaveCount(0);
    await expect(check).toBeFocused();
    await expect(page.getByText("My store: Demo · Belgrade Central", { exact: false })).toBeVisible();
    await page.reload();
    await expect(page.getByText("My store: Demo · Belgrade Central", { exact: false })).toBeVisible();
  });
  test("searches products and checks selected hoodie variants on the stores page", async ({ page }) => {
    await page.goto("/stores");
    await page.getByLabel("Product name or SKU").fill("STORE-DEMO-HOODIE");
    await page.getByRole("button", { name: "Search", exact: true }).click();
    await page.getByRole("button", { name: "Store demo hoodie · STORE-DEMO-HOODIE", exact: true }).click();
    const variant = page.getByRole("combobox", { name: "Variant", exact: true });
    await variant.selectOption({ label: "Store demo hoodie / Small · STORE-DEMO-HOODIE-S" });
    await page.getByLabel("In stock only").check();
    await expect(page.getByText("Demo · Belgrade Central")).toBeVisible();
    await expect(page.getByText("Demo · Novi Sad")).toHaveCount(0);
    await variant.selectOption({ label: "Store demo hoodie / Large · STORE-DEMO-HOODIE-L" });
    await expect(page.getByText("Demo · Novi Sad")).toBeVisible();
    await expect(page.getByText("Demo · Belgrade Central")).toHaveCount(0);
  });
  test("checks fractional stock in a mobile dialog without horizontal overflow", async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto("/store-demo-fabric");
    await page.getByRole("button", { name: "Check availability in stores" }).click();
    const dialog = page.getByRole("dialog", { name: "Check availability in stores" });
    await expect(dialog).toBeVisible();
    await expect(dialog.getByText("Low stock", { exact: true }).first()).toBeVisible();
    expect(await dialog.evaluate(element => element.scrollWidth <= element.clientWidth)).toBe(true);
    await dialog.getByRole("button", { name: "Close", exact: true }).click();
    await expect(dialog).toHaveCount(0);
  });

});
