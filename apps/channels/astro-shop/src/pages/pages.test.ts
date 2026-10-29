import { experimental_AstroContainer as AstroContainer } from "astro/container";
import { getContainerRenderer } from "@astrojs/react/container-renderer";
import { loadRenderers } from "astro:container";
import { JSDOM } from "jsdom";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

// Pages rendered for real, with the backend answered per operation - what the head, the JSON-LD and the
// cache tags look like is decided here, not in a browser
const gql = vi.fn();
vi.mock("../lib/gql", async (orig) => ({ ...(await orig<typeof import("../lib/gql")>()), gql }));

const channel = {
  name: "Test Shop",
  defaultLocale: "en",
  publicUrl: "https://shop.test",
  currency: { code: "USD", symbol: "", symbolPosition: "prefix", decimals: 2 },
};
const shirt = {
  id: "p1",
  url: "casual-t-shirt",
  sku: "TSHIRT",
  name: "Casual T-Shirt",
  kind: "SIMPLE",
  inStock: true,
  price: 1500,
  compareAtPrice: null,
  onSale: false,
  minOrderQty: null,
  quantityStep: 1,
  maxOrderQty: null,
  metaTitle: null,
  metaDescription: null,
  description: { root: { children: [{ children: [{ text: "Soft cotton tee." }] }] } },
  mainImage: null,
  children: [],
};

type Answers = Record<string, unknown>;
function backend(answers: Answers) {
  gql.mockImplementation(async (doc: { definitions: { name?: { value: string } }[] }) => {
    const op = doc.definitions[0]?.name?.value ?? "";
    if (op === "CurrentChannel") return { currentChannel: channel };
    if (op in answers) return answers[op];
    throw new Error(`unexpected operation ${op}`);
  });
}

let container: AstroContainer;
const pages = {
  index: () => import("./index.astro"),
  "[...slug]": () => import("./[...slug].astro"),
  cart: () => import("./cart.astro"),
};

beforeAll(async () => {
  container = await AstroContainer.create({ renderers: await loadRenderers([getContainerRenderer()]) });
  // What the catch-all rewrites to when the backend knows no product at a path
  container.insertPageRoute("/404", (await import("./404.astro")).default);
});

beforeEach(async () => {
  gql.mockReset();
  (await import("../lib/channel")).forgetChannel();
});

async function render(page: keyof typeof pages, path: string, params: Record<string, string> = {}) {
  const mod = await pages[page]();
  const res = await container.renderToResponse(mod.default, {
    request: new Request(`http://localhost${path}`),
    params,
    locals: {},
    partial: false,
  } as never);
  return { res, html: await res.text() };
}

// Asserted on the parsed page, so how a tag is spelled (React's "/>", attribute order) doesn't matter
const dom = (html: string) => new JSDOM(html).window.document;
const attr = (html: string, selector: string, name: string) => dom(html).querySelector(selector)?.getAttribute(name);

function jsonLdOf(html: string): { "@type": string; [k: string]: unknown }[] {
  return [...html.matchAll(/<script type="application\/ld\+json">(.*?)<\/script>/g)].map((m) => JSON.parse(m[1]!));
}

describe("home page", () => {
  it("lists products with SEO head and WebSite JSON-LD", async () => {
    backend({ HomeProducts: { browseProducts: { items: [shirt, { ...shirt, id: "p2", url: null, name: "Hidden" }] } } });
    const { html } = await render("index", "/");
    expect(html).toContain('href="/casual-t-shirt"');
    expect(html).not.toContain("Hidden");
    expect(html).toContain("<title>Test Shop</title>");
    expect(attr(html, "link[rel=canonical]", "href")).toBe("https://shop.test/");
    expect(html).toContain('content="index, follow, max-image-preview:large"');
    expect(jsonLdOf(html).map((d) => d["@type"])).toEqual(["WebSite"]);
  });
});

describe("product page", () => {
  it("renders the product with Product and BreadcrumbList JSON-LD", async () => {
    backend({ Route: { route: { kind: "PRODUCT", entityId: "p1" } }, ProductPage: { product: shirt } });
    const { res, html } = await render("[...slug]", "/casual-t-shirt", { slug: "casual-t-shirt" });
    expect(res.status).toBe(200);
    expect(html).toContain("<title>Casual T-Shirt · Test Shop</title>");
    expect(attr(html, "meta[name=description]", "content")).toBe("Soft cotton tee.");
    expect(attr(html, "meta[property='og:type']", "content")).toBe("product");
    expect(attr(html, "link[rel=canonical]", "href")).toBe("https://shop.test/casual-t-shirt");
    const [product, crumbs] = jsonLdOf(html);
    expect(product).toMatchObject({ "@type": "Product", sku: "TSHIRT", offers: { price: "15.00", priceCurrency: "USD" } });
    expect(crumbs?.["@type"]).toBe("BreadcrumbList");
    expect(html).toMatch(/<form method="post" action="\/cart"/);
    expect(html).toContain('name="sku" value="TSHIRT"');
  });

  it("offers in-stock variants and disables sold-out ones", async () => {
    const hoodie = {
      ...shirt,
      kind: "CONFIGURABLE",
      name: "Hoodie",
      children: [
        { id: "v1", sku: "H-S", name: "Hoodie / Small", inStock: false, price: 4900 },
        { id: "v2", sku: "H-L", name: "Hoodie / Large", inStock: true, price: 4900 },
      ],
    };
    backend({ Route: { route: { kind: "PRODUCT", entityId: "p1" } }, ProductPage: { product: hoodie } });
    const { html } = await render("[...slug]", "/hoodie", { slug: "hoodie" });
    const doc = dom(html);
    expect(doc.querySelector<HTMLInputElement>("input[value=H-S]")?.disabled).toBe(true);
    expect(doc.querySelector<HTMLInputElement>("input[value=H-L]")?.hasAttribute("checked")).toBe(true);
    expect(html).toContain(">Small<");
  });

  it("answers an unknown path with a 404 kept out of the index", async () => {
    backend({ Route: { route: null } });
    const { res, html } = await render("[...slug]", "/nope", { slug: "nope" });
    expect(res.status).toBe(404);
    expect(html).toContain('content="noindex, follow"');
    expect(dom(html).querySelector("h1")?.textContent).toBe("We couldn't find that page");
  });

  it("follows the backend's redirects", async () => {
    backend({ Route: { route: { kind: "PRODUCT", entityId: null, redirectTo: "/new-shirt", redirectStatusCode: 301 } } });
    const { res } = await render("[...slug]", "/old-shirt", { slug: "old-shirt" });
    expect(res.status).toBe(301);
    expect(res.headers.get("location")).toBe("/new-shirt");
  });
});

describe("cart page", () => {
  it("shows an empty cart with no cookie and is noindex", async () => {
    backend({});
    const { html } = await render("cart", "/cart");
    expect(html).toContain("Your cart is empty");
    expect(html).toContain('content="noindex, follow"');
  });
});

describe("view transitions", () => {
  it("browsing pages opt in, the form flows don't", async () => {
    backend({ HomeProducts: { browseProducts: { items: [] } } });
    expect((await render("index", "/")).html).toContain("@view-transition");
    expect((await render("cart", "/cart")).html).not.toContain("@view-transition");
  });
});
