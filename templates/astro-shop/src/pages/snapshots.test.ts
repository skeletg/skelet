import { experimental_AstroContainer as AstroContainer } from "astro/container";
import { getContainerRenderer } from "@astrojs/react/container-renderer";
import { loadRenderers } from "astro:container";
import { JSDOM } from "jsdom";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

import { GqlError } from "../lib/gql";

// Characterization: every route and state rendered for real and frozen as normalized HTML, so moving markup
// between .astro and React can be checked to change nothing a shopper, a crawler or enhance.ts sees.
// Normalized away: comments, whitespace-only text, attribute order and quoting, entity spelling, the footer year
const gql = vi.hoisted(() => vi.fn());
vi.mock("../lib/gql", async (orig) => ({ ...(await orig<typeof import("../lib/gql")>()), gql }));
// The core's reads the pages make outside the shop's own operations
const core = vi.hoisted(() => ({ getChannel: vi.fn(), getCart: vi.fn(), getOwnCartId: vi.fn() }));
vi.mock("@skelet/storefront-core/channel/read", () => ({ getChannel: core.getChannel }));
vi.mock("@skelet/storefront-core/cart/read", () => ({ CART_COOKIE: "storefront_cart", getCart: core.getCart, getOwnCartId: core.getOwnCartId }));

const channel = {
  name: "Test Shop",
  defaultLocale: "en",
  publicUrl: "https://shop.test",
  currency: { code: "USD", symbol: "", symbolPosition: "prefix", decimals: 2 },
};
const image = { url: "https://cdn.test/shirt.jpg", altText: "A grey tee", width: 800, height: 1000 };
const shirt = {
  id: "p1",
  url: "casual-t-shirt",
  sku: "TSHIRT",
  name: "Casual T-Shirt",
  kind: "SIMPLE",
  inStock: true,
  price: 1500,
  compareAtPrice: 2000,
  onSale: true,
  minOrderQty: null,
  quantityStep: 1,
  maxOrderQty: 5,
  metaTitle: null,
  metaDescription: null,
  description: { root: { children: [{ children: [{ text: "Soft cotton tee." }] }, { children: [{ text: "Washes well." }] }] } },
  mainImage: image,
  children: [],
};
const hoodie = {
  ...shirt,
  id: "p2",
  url: "hoodie",
  sku: "HOODIE",
  kind: "CONFIGURABLE",
  name: "Hoodie",
  compareAtPrice: null,
  onSale: false,
  mainImage: null,
  description: null,
  children: [
    { id: "v1", sku: "H-S", name: "Hoodie / Small", inStock: false, price: 4900 },
    { id: "v2", sku: "H-L", name: "Hoodie / Large", inStock: true, price: 4900 },
  ],
};
const lines = [
  { id: "l1", name: "Casual T-Shirt", sku: "TSHIRT", imageUrl: "https://cdn.test/shirt.jpg", unitPrice: 1500, quantity: 2, totalPrice: 3000 },
  { id: "l2", name: "Mug", sku: "MUG", imageUrl: null, unitPrice: 900, quantity: 1, totalPrice: 900 },
];
const cart = { id: "c1", subtotal: 3900, totalDiscount: 400, lines };
const choice = (n: number, price: number) => ({
  optionId: `o${n}`,
  optionName: `Option ${n}`,
  kind: "ship_to_address",
  carrierSlug: "post",
  methodCode: `m${n}`,
  methodName: n === 1 ? "Standard" : "",
  priceCents: price,
  estimatedArrival: n === 1 ? "2026-11-03T12:00:00Z" : null,
});
const checkout = (destination: { countryCode: string; postalCode: string } | null) => ({
  cart: {
    ...cart,
    checkout: {
      email: "ana@example.test",
      paymentProviderSlug: null,
      destination,
      shippingAddress: destination
        ? { firstName: "Ana", lastName: "Petrović", street1: "Main 1", street2: null, city: "Belgrade", state: null, phone: null }
        : null,
      deliveryGroups: destination
        ? [{ key: "g1", choices: [choice(1, 0), choice(2, 990), { ...choice(3, 500), kind: "pickup_point" }], selection: null }]
        : [],
      requirements: [],
      totals: { subtotalCents: 3900, discountCents: 400, shippingCents: destination ? 990 : 0, taxCents: 120, totalCents: 4610 },
    },
  },
});
const countries = { countries: [{ code: "RS", name: "Serbia" }, { code: "DE", name: "Germany" }] };
const payments = {
  paymentMethods: [
    { providerSlug: "cod", displayName: "Cash on delivery", instructions: "Pay the courier." },
    { providerSlug: "bank", displayName: "Bank transfer", instructions: "" },
  ],
};
const rs = { countryCode: "RS", postalCode: "11000" };

type Answers = Record<string, unknown>;
function backend(answers: Answers) {
  gql.mockImplementation(async (doc: { definitions: { name?: { value: string } }[] }) => {
    const op = doc.definitions[0]?.name?.value ?? "";
    if (op === "CurrentChannel") return { currentChannel: channel };
    if (op in answers) {
      const a = answers[op];
      if (a instanceof Error) throw a;
      return a;
    }
    throw new Error(`unexpected operation ${op}`);
  });
}

const pages = {
  index: () => import("./index.astro"),
  "[...slug]": () => import("./[...slug].astro"),
  cart: () => import("./cart.astro"),
  checkout: () => import("./checkout.astro"),
  "order-placed": () => import("./order-placed.astro"),
};

let container: AstroContainer;
beforeAll(async () => {
  container = await AstroContainer.create({ renderers: await loadRenderers([getContainerRenderer()]) });
  container.insertPageRoute("/404", (await import("./404.astro")).default);
});

beforeEach(async () => {
  gql.mockReset();
  core.getChannel.mockReset().mockResolvedValue(channel);
  core.getCart.mockReset().mockResolvedValue(null);
  core.getOwnCartId.mockReset().mockResolvedValue(null);
});

// The shopper's cart, as the core reads it from its own cookie
function renderCheckout(form?: Record<string, string>) {
  core.getOwnCartId.mockResolvedValue("c1");
  return render("checkout", "/checkout", { form });
}

type Opts = { params?: Record<string, string>; cookie?: string; form?: Record<string, string> };
async function render(page: keyof typeof pages, path: string, { params = {}, cookie, form }: Opts = {}) {
  const mod = await pages[page]();
  const headers = new Headers(cookie ? { cookie } : {});
  const body = form ? new URLSearchParams(form) : undefined;
  if (body) headers.set("content-type", "application/x-www-form-urlencoded");
  const res = await container.renderToResponse(mod.default, {
    request: new Request(`http://localhost${path}`, { method: form ? "POST" : "GET", headers, body }),
    params,
    locals: {},
    partial: false,
  } as never);
  return { status: res.status, html: normalize(await res.text()) };
}

// Where a leading or trailing space is part of the line - inside anything else, the browser doesn't draw it
const inline = new Set(["a", "b", "em", "i", "label", "s", "small", "span", "strong"]);

// One element per line, attributes sorted, so a diff points at exactly what moved. Comments go and the text around
// them merges, so React's <!-- --> separators and Astro's template comments both drop out
function normalize(html: string): string {
  // The dev script src carries the checkout's absolute path
  const { document, Node, NodeFilter } = new JSDOM(html.replaceAll(process.cwd(), "<root>")).window;
  const comments = document.createTreeWalker(document, NodeFilter.SHOW_COMMENT);
  const dead: Node[] = [];
  while (comments.nextNode()) dead.push(comments.currentNode);
  dead.forEach((c) => c.parentNode?.removeChild(c));
  document.documentElement.normalize();

  const out: string[] = [];
  const walk = (node: Node, depth: number) => {
    const pad = "  ".repeat(depth);
    if (node.nodeType === Node.TEXT_NODE) {
      let text = (node.textContent ?? "").replace(/\s+/g, " ");
      const parent = node.parentElement?.tagName.toLowerCase() ?? "";
      if (!inline.has(parent)) {
        if (!node.previousSibling) text = text.trimStart();
        if (!node.nextSibling) text = text.trimEnd();
      }
      if (text.trim()) out.push(pad + JSON.stringify(text.replace(/© \d{4}/, "© YEAR")));
      return;
    }
    if (node.nodeType !== Node.ELEMENT_NODE) return;
    const el = node as Element;
    const attrs = [...el.attributes]
      .map((a) => (a.value === "" ? a.name : `${a.name}=${JSON.stringify(a.value)}`))
      .sort()
      .join(" ");
    const tag = el.tagName.toLowerCase();
    out.push(`${pad}<${tag}${attrs ? " " + attrs : ""}>`);
    if (tag === "script" || tag === "style") {
      const raw = (el.textContent ?? "").trim();
      if (raw) out.push(pad + "  " + raw.replace(/\s+/g, " "));
      return;
    }
    const kids = tag === "template" ? (el as HTMLTemplateElement).content.childNodes : el.childNodes;
    kids.forEach((k) => walk(k, depth + 1));
  };
  walk(document.documentElement, 0);
  return out.join("\n") + "\n";
}

const snap = (name: string) => `./__snapshots__/${name}.html`;

describe("page output", () => {
  it("home", async () => {
    backend({ HomeProducts: { browseProducts: { items: [shirt, hoodie, { ...shirt, id: "p3", url: null, name: "Hidden" }] } } });
    const { status, html } = await render("index", "/");
    expect(status).toBe(200);
    await expect(html).toMatchFileSnapshot(snap("home"));
  });

  it("home, empty catalog", async () => {
    backend({ HomeProducts: { browseProducts: { items: [] } } });
    await expect((await render("index", "/")).html).toMatchFileSnapshot(snap("home-empty"));
  });

  it("product, simple on sale", async () => {
    backend({ Route: { route: { kind: "PRODUCT", entityId: "p1" } }, ProductPage: { product: shirt } });
    const { status, html } = await render("[...slug]", "/casual-t-shirt", { params: { slug: "casual-t-shirt" } });
    expect(status).toBe(200);
    await expect(html).toMatchFileSnapshot(snap("product-simple"));
  });

  it("product, configurable with a sold-out variant", async () => {
    backend({ Route: { route: { kind: "PRODUCT", entityId: "p2" } }, ProductPage: { product: hoodie } });
    await expect((await render("[...slug]", "/hoodie", { params: { slug: "hoodie" } })).html).toMatchFileSnapshot(snap("product-configurable"));
  });

  it("product, sold out", async () => {
    backend({ Route: { route: { kind: "PRODUCT", entityId: "p1" } }, ProductPage: { product: { ...shirt, inStock: false } } });
    await expect((await render("[...slug]", "/casual-t-shirt", { params: { slug: "casual-t-shirt" } })).html).toMatchFileSnapshot(snap("product-sold-out"));
  });

  it("product, not sellable here", async () => {
    backend({ Route: { route: { kind: "PRODUCT", entityId: "p1" } }, ProductPage: { product: { ...shirt, kind: "BUNDLE" } } });
    await expect((await render("[...slug]", "/casual-t-shirt", { params: { slug: "casual-t-shirt" } })).html).toMatchFileSnapshot(snap("product-unsellable"));
  });

  it("404", async () => {
    backend({ Route: { route: null } });
    const { status, html } = await render("[...slug]", "/nope", { params: { slug: "nope" } });
    expect(status).toBe(404);
    await expect(html).toMatchFileSnapshot(snap("404"));
  });

  it("cart, empty", async () => {
    backend({});
    await expect((await render("cart", "/cart")).html).toMatchFileSnapshot(snap("cart-empty"));
  });

  it("cart, filled with a flash message", async () => {
    core.getCart.mockResolvedValue(cart);
    const { html } = await render("cart", "/cart", { cookie: "flash=Only 2 left" });
    await expect(html).toMatchFileSnapshot(snap("cart-filled"));
  });

  it("checkout, step 1", async () => {
    backend({ Checkout: checkout(null), Countries: countries });
    await expect((await renderCheckout()).html).toMatchFileSnapshot(snap("checkout-destination"));
  });

  it("checkout, step 2", async () => {
    backend({ Checkout: checkout(rs), Countries: countries, PaymentOptions: payments });
    await expect((await renderCheckout()).html).toMatchFileSnapshot(snap("checkout-details"));
  });

  it("checkout, step 2 with nothing deliverable or payable", async () => {
    const c = checkout(rs);
    c.cart.checkout.deliveryGroups = [{ key: "g1", choices: [], selection: null }];
    backend({ Checkout: c, Countries: countries, PaymentOptions: { paymentMethods: [] } });
    await expect((await renderCheckout()).html).toMatchFileSnapshot(snap("checkout-blocked"));
  });

  it("checkout, an error re-render keeps what was typed", async () => {
    backend({
      Checkout: checkout(rs),
      Countries: countries,
      PaymentOptions: payments,
      SetDelivery: { setCartDelivery: { id: "c1" } },
      SetDetails: new GqlError("Enter a valid email", "VALIDATION"),
    });
    const form = { intent: "place", group: "g1", "delivery:g1": "o2|post|m2", email: "nope", firstName: "Mila", payment: "bank", acceptTerms: "on" };
    const { html } = await renderCheckout(form);
    expect(html).toContain("Enter a valid email");
    await expect(html).toMatchFileSnapshot(snap("checkout-error"));
  });

  it("order placed", async () => {
    backend({});
    await expect((await render("order-placed", "/order-placed?n=SO-1001")).html).toMatchFileSnapshot(snap("order-placed"));
  });
});
