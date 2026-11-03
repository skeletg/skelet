import { expect, test } from "@playwright/test";

import { jsonLd, openBuyableProduct } from "./helpers";

test.describe("browsing", () => {
  test("home lists products that link to their pages", async ({ page }) => {
    const res = await page.goto("/");
    expect(res?.status()).toBe(200);
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    const cards = page.locator("#products li a");
    expect(await cards.count()).toBeGreaterThan(0);
    const first = cards.first();
    const href = await first.getAttribute("href");
    await first.click();
    await expect(page).toHaveURL(new RegExp(`${href}$`));
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
  });

  test("home has a canonical URL, a description and WebSite JSON-LD", async ({ page }) => {
    await page.goto("/");
    await expect(page.locator('link[rel="canonical"]')).toHaveAttribute("href", /\/$/);
    await expect(page.locator('meta[name="description"]')).toHaveAttribute("content", /\S{10,}/);
    await expect(page.locator('meta[name="robots"]')).toHaveAttribute("content", /^index/);
    expect((await jsonLd(page)).map((d) => d["@type"])).toContain("WebSite");
  });

  test("a product page carries Product JSON-LD that matches what's shown", async ({ page }) => {
    await openBuyableProduct(page);
    const [product] = (await jsonLd(page)).filter((d) => d["@type"] === "Product");
    expect(product).toBeTruthy();
    await expect(page.getByRole("heading", { level: 1 })).toHaveText(String(product!.name));
    expect(product!.offers).toMatchObject({ availability: "https://schema.org/InStock" });
    await expect(page.locator('meta[property="og:type"]')).toHaveAttribute("content", "product");
    await expect(page.getByRole("navigation", { name: "Breadcrumb" })).toBeVisible();
  });

  test("an unknown page is a 404 kept out of the index", async ({ page }) => {
    const res = await page.goto("/no-such-product-e2e");
    expect(res?.status()).toBe(404);
    await expect(page.getByRole("heading", { name: "We couldn't find that page" })).toBeVisible();
    await expect(page.locator('meta[name="robots"]')).toHaveAttribute("content", /noindex/);
  });

  test("robots.txt and sitemap.xml describe the shop", async ({ page, request }) => {
    const robots = await (await request.get("/robots.txt")).text();
    expect(robots).toContain("Disallow: /checkout");
    expect(robots).toMatch(/Sitemap: https?:\/\/.+\/sitemap\.xml/);

    const sitemap = await request.get("/sitemap.xml");
    expect(sitemap.headers()["content-type"]).toContain("xml");
    const locs = [...(await sitemap.text()).matchAll(/<loc>(.*?)<\/loc>/g)].map((m) => m[1]!);
    await page.goto("/");
    const cards = await page.locator("#products li a").count();
    expect(locs.length).toBe(cards + 1);
  });
});

test.describe("motion", () => {
  test("cards below the fold reveal as they scroll in, and end fully visible", async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 500 });
    await page.goto("/");
    const last = page.locator("#products li a").last();
    await last.scrollIntoViewIfNeeded();
    await expect.poll(() => last.evaluate((el) => getComputedStyle(el).opacity)).toBe("1");
  });

  test("reduced motion hides nothing, ever", async ({ browser }) => {
    const ctx = await browser.newContext({ reducedMotion: "reduce", viewport: { width: 390, height: 500 } });
    const page = await ctx.newPage();
    await page.goto("/");
    const hidden = await page.locator("[data-reveal]").evaluateAll((els) => els.filter((e) => getComputedStyle(e).opacity !== "1").length);
    expect(hidden).toBe(0);
    await ctx.close();
  });

  test("the product image keeps one view-transition name from grid to page", async ({ page }) => {
    await page.goto("/");
    const card = page.locator("#products li a").first();
    const name = await card.locator("[style*='view-transition-name']").evaluate((el) => (el as HTMLElement).style.viewTransitionName);
    expect(name).toMatch(/^product-/);
    await card.click();
    await expect(page.locator(`[style*='${name}']`)).toHaveCount(1);
  });
});
