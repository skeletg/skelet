/* eslint-disable @next/next/no-html-link-for-pages -- plain anchors stand in for the cards' links */
import { act, fireEvent, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { ItemListEvents } from "./ItemListEvents";

const beacons: { name: string; ecommerce: { item_list_id?: string; items: { item_id: string; index?: number }[] } }[] = [];

beforeEach(() => {
  beacons.length = 0;
  Object.defineProperty(navigator, "sendBeacon", {
    configurable: true,
    value: (_url: string, blob: Blob) => {
      void blob.text().then((t) => beacons.push(JSON.parse(t)));
      return true;
    },
  });
});

afterEach(() => {
  document.cookie = "storefront_consent=; max-age=0; path=/";
});

const flush = () => act(() => new Promise((r) => setTimeout(r, 5)));
const items = [
  { item_id: "SKU-1", index: 0 },
  { item_id: "SKU-2", index: 1 },
];
const grid = () =>
  render(
    <ItemListEvents list={{ id: "/c/mugs", name: "/c/mugs" }} currency="EUR" items={items as never}>
      <ul>
        <li data-item-index={0}>
          <a href="/p/1">one</a>
        </li>
        <li data-item-index={1}>
          <a href="/p/2">
            <span>two</span>
          </a>
          <button>wishlist</button>
        </li>
      </ul>
    </ItemListEvents>,
  );

describe("ItemListEvents", () => {
  it("reports the list once and the product whose link was clicked", async () => {
    document.cookie = "storefront_consent=v2%3Aa1.d0.p0; path=/";
    const { getByText } = grid();
    fireEvent.click(getByText("two"));
    fireEvent.click(getByText("wishlist")); // not a link to the product
    await flush();
    expect(beacons.map((b) => b.name)).toEqual(["view_item_list", "select_item"]);
    expect(beacons[1].ecommerce.item_list_id).toBe("/c/mugs");
    expect(beacons[1].ecommerce.items).toEqual([{ item_id: "SKU-2", index: 1 }]);
  });

  it("sends nothing before the visitor allowed any category", async () => {
    const { getByText } = grid();
    fireEvent.click(getByText("one"));
    await flush();
    expect(beacons).toEqual([]);
  });
});
