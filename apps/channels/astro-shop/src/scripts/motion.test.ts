// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from "vitest";

// jsdom has neither the Web Animations API nor IntersectionObserver - the library is stubbed, the decisions are tested
const animate = vi.fn(() => Promise.resolve());
const inView = vi.fn();
vi.mock("motion/mini", () => ({ animate }));
vi.mock("motion", () => ({ inView }));

const { initMotion } = await import("./motion");

function setup(reduced: boolean) {
  document.body.innerHTML = `
    <span data-cart-count>2</span>
    <a data-reveal id="above"></a>
    <a data-reveal id="below"></a>`;
  const top = (id: string) => (id === "below" ? 2000 : 100);
  for (const el of document.querySelectorAll<HTMLElement>("[data-reveal]")) {
    el.getBoundingClientRect = () => ({ top: top(el.id) }) as DOMRect;
  }
  window.matchMedia = ((q: string) => ({ matches: reduced && q.includes("reduce") })) as typeof window.matchMedia;
}

beforeEach(() => {
  animate.mockClear();
  inView.mockClear();
  document.cookie = "cart_bump=; Max-Age=0; Path=/";
});

describe("initMotion", () => {
  it("hides only what starts below the fold, and reveals it on scroll", async () => {
    setup(false);
    initMotion();
    const above = document.getElementById("above")!;
    const below = document.getElementById("below")!;
    expect(above.style.opacity).toBe("");
    expect(below.style.opacity).toBe("0");
    expect(inView).toHaveBeenCalledWith([below], expect.any(Function), expect.any(Object));

    const reveal = inView.mock.calls[0]![1] as (el: Element) => void;
    reveal(below);
    expect(animate).toHaveBeenCalledWith(below, expect.objectContaining({ opacity: [0, 1] }), expect.any(Object));
    await Promise.resolve();
    await Promise.resolve();
    expect(below.style.opacity).toBe("");
  });

  it("does nothing at all for reduced motion", () => {
    setup(true);
    document.cookie = "cart_bump=1; Path=/";
    initMotion();
    expect(document.getElementById("below")!.style.opacity).toBe("");
    expect(inView).not.toHaveBeenCalled();
    expect(animate).not.toHaveBeenCalled();
  });

  it("pops the cart badge once after an add", () => {
    setup(false);
    document.cookie = "cart_bump=1; Path=/";
    initMotion();
    const badge = document.querySelector("[data-cart-count]");
    expect(animate).toHaveBeenCalledWith(badge, expect.objectContaining({ transform: expect.any(Array) }), expect.any(Object));
    expect(document.cookie).not.toContain("cart_bump=1");
  });
});
