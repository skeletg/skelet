// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { Choice } from "./Choice";
import { Price } from "./Price";
import { ProductCard, type CardProduct } from "./ProductCard";
import { ProductImage } from "./ProductImage";
import { Button, ButtonLink, Notice } from "./ui";

afterEach(cleanup);

const usd = { code: "USD", symbol: "", decimals: 2 };
const product: CardProduct = { id: "p1", url: "casual-t-shirt", name: "Casual T-Shirt", inStock: true, price: 1500 };

describe("Price", () => {
  it("shows the amount alone when not on sale", () => {
    const { container } = render(<Price amount={1500} currency={usd} />);
    expect(container.textContent).toBe("$15.00");
    expect(container.querySelector("s")).toBeNull();
  });

  it("strikes the old price through on sale, with a screen-reader cue", () => {
    const { container } = render(<Price amount={1200} compareAt={1500} onSale currency={usd} />);
    expect(container.querySelector("s")?.textContent).toBe("$15.00");
    expect(container.textContent).toContain(", was ");
  });

  it("ignores a compare-at price that isn't higher", () => {
    const { container } = render(<Price amount={1500} compareAt={1500} onSale currency={usd} />);
    expect(container.querySelector("s")).toBeNull();
  });
});

describe("ProductImage", () => {
  it("renders the photo lazily, with its alt text and size", () => {
    render(<ProductImage name="Mug" image={{ url: "https://cdn.test/m.jpg", altText: "A mug", width: 600, height: 750 }} />);
    const img = screen.getByRole("img", { name: "A mug" });
    expect(img.getAttribute("loading")).toBe("lazy");
    expect(img.getAttribute("width")).toBe("600");
  });

  it("loads the priority (LCP) image eagerly at high priority", () => {
    render(<ProductImage name="Mug" image={{ url: "https://cdn.test/m.jpg" }} priority />);
    const img = screen.getByRole("img");
    expect(img.getAttribute("loading")).toBe("eager");
    expect(img.getAttribute("fetchpriority")).toBe("high");
  });

  it("falls back to a decorative initial and carries the view-transition name", () => {
    const { container } = render(<ProductImage name="Mug" transitionName="product-p1" />);
    expect(screen.queryByRole("img")).toBeNull();
    expect(container.textContent).toBe("M");
    expect((container.firstChild as HTMLElement).style.viewTransitionName).toBe("product-p1");
  });
});

describe("ProductCard", () => {
  it("links to the product's own URL with its name and price", () => {
    render(<ProductCard product={product} currency={usd} />);
    const link = screen.getByRole("link");
    expect(link.getAttribute("href")).toBe("/casual-t-shirt");
    expect(screen.getByRole("heading", { name: "Casual T-Shirt" })).toBeTruthy();
    expect(link.textContent).toContain("$15.00");
    expect(link.hasAttribute("data-reveal")).toBe(true);
  });

  it("badges sold-out and sale products", () => {
    const { rerender } = render(<ProductCard product={{ ...product, inStock: false }} currency={usd} />);
    expect(screen.getByText("Sold out")).toBeTruthy();
    rerender(<ProductCard product={{ ...product, onSale: true, compareAtPrice: 2000 }} currency={usd} />);
    expect(screen.getByText("Sale")).toBeTruthy();
  });
});

describe("ui", () => {
  it("Button forwards native attributes", () => {
    render(<Button type="submit" disabled>Add to cart</Button>);
    const btn = screen.getByRole("button", { name: "Add to cart" }) as HTMLButtonElement;
    expect(btn.type).toBe("submit");
    expect(btn.disabled).toBe(true);
  });

  it("ButtonLink is a real link", () => {
    render(<ButtonLink href="/checkout">Checkout</ButtonLink>);
    expect(screen.getByRole("link", { name: "Checkout" }).getAttribute("href")).toBe("/checkout");
  });

  it("Notice is announced", () => {
    render(<Notice>Out of stock</Notice>);
    expect(screen.getByRole("alert").textContent).toBe("Out of stock");
  });
});

describe("Choice", () => {
  it("is a required radio named by its content, wired to autosubmit", () => {
    render(<Choice name="sku" value="H-L" checked autosubmit="update-shipping">Large</Choice>);
    const radio = screen.getByRole("radio", { name: "Large" }) as HTMLInputElement;
    expect(radio.checked).toBe(true);
    expect(radio.required).toBe(true);
    expect(radio.dataset.autosubmit).toBe("update-shipping");
  });

  it("can be sold out", () => {
    render(<Choice name="sku" value="H-S" disabled>Small</Choice>);
    expect((screen.getByRole("radio", { name: "Small" }) as HTMLInputElement).disabled).toBe(true);
  });
});
