"use client";

import type { ReactNode } from "react";

import { track } from "@skelet/storefront-core/track/events";
import type { ItemList, ga4Item } from "@skelet/storefront-core/track/items";
import { trackingAllowed } from "@skelet/storefront-react/consent/client";

import { AnalyticsEvent } from "./AnalyticsEvent";

type Item = ReturnType<typeof ga4Item>;

// GA4's view_item_list for a grid as it renders, and select_item for the product clicked in it. Each entry the grid
// renders carries data-item-index, so a click anywhere in a card names its item without the cards knowing about this
export function ItemListEvents({ list, currency, items, children }: { list: ItemList; currency: string; items: Item[]; children: ReactNode }) {
  const onClick = (e: React.MouseEvent) => {
    const el = (e.target as Element).closest?.("a[href]")?.closest("[data-item-index]");
    const item = el ? items[Number(el.getAttribute("data-item-index"))] : undefined;
    if (!item || !trackingAllowed()) return;
    track("select_item", crypto.randomUUID(), { ecommerce: { currency, item_list_id: list.id, item_list_name: list.name, items: [item] } });
  };
  return (
    <div onClickCapture={onClick}>
      <AnalyticsEvent
        event="view_item_list"
        payload={{ ecommerce: { currency, item_list_id: list.id, item_list_name: list.name, items } }}
      />
      {children}
    </div>
  );
}
