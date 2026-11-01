import { act, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

let pathname = "/en";
vi.mock("next/navigation", () => ({ usePathname: () => pathname }));

const { PageViewBeacon } = await import("@/app/components/PageViewBeacon");

const beacons: { url: string; body: Record<string, unknown> }[] = [];

beforeEach(() => {
  beacons.length = 0;
  Object.defineProperty(navigator, "sendBeacon", {
    configurable: true,
    value: (url: string, blob: Blob) => {
      void blob.text().then((t) => beacons.push({ url, body: JSON.parse(t) }));
      return true;
    },
  });
  vi.stubGlobal("requestAnimationFrame", (cb: FrameRequestCallback) => setTimeout(() => cb(0), 0) as unknown as number);
  vi.stubGlobal("cancelAnimationFrame", (id: number) => clearTimeout(id));
});

afterEach(() => {
  document.cookie = "storefront_consent=; max-age=0; path=/";
  vi.unstubAllGlobals();
});

const flush = () => act(() => new Promise((r) => setTimeout(r, 5)));

describe("PageViewBeacon", () => {
  it("sends a page view per navigation, with the page title, once any category is granted", async () => {
    document.cookie = "storefront_consent=v2%3Aa0.d1.p0; path=/";
    document.title = "Home";
    const { rerender } = render(<PageViewBeacon />);
    await flush();
    pathname = "/en/products/mug";
    document.title = "Blue mug";
    rerender(<PageViewBeacon />);
    await flush();
    await flush();
    expect(beacons.map((b) => [b.url, b.body.name, b.body.page_title])).toEqual([
      ["/t/e", "page_view", "Home"],
      ["/t/e", "page_view", "Blue mug"],
    ]);
  });

  it("sends nothing before the visitor allows anything", async () => {
    pathname = "/en/other";
    render(<PageViewBeacon />);
    await flush();
    expect(beacons).toEqual([]);
  });
});
