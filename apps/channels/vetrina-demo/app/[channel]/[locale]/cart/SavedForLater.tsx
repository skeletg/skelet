"use client";

import { useTransition } from "react";

import {
  moveSavedToCartAction,
  removeSavedItemAction,
  saveForLaterAction,
} from "@/app/lib/cart/actions";
import { useCartStore } from "@/app/lib/cart/store";
import { displayName } from "@/app/lib/display";
import type { SavedItem } from "@/app/lib/cart/types";
import { mediaThumb } from "@/app/lib/media-thumb";

export function SaveForLaterButton({ lineId }: { lineId: string }) {
  const [pending, start] = useTransition();
  const { applyServerCart } = useCartStore();
  return (
    <button
      type="button"
      disabled={pending}
      onClick={() =>
        start(async () => {
          const cart = await saveForLaterAction(lineId);
          applyServerCart(cart);
        })
      }
      className="text-xs text-muted-foreground underline-offset-2 transition-colors hover:text-foreground hover:underline disabled:opacity-50"
    >
      {pending ? "Saving…" : "Save for later"}
    </button>
  );
}

export function SavedForLater() {
  const { cart } = useCartStore();
  const items = cart?.savedItems ?? [];
  if (items.length === 0) return null;
  return (
    <section className="mt-12">
      <h2 className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
        Saved for later ({items.length})
      </h2>
      <ul className="glass mt-3 divide-y divide-border rounded-xl px-5">
        {items.map((it) => (
          <SavedRow key={it.id} item={it} />
        ))}
      </ul>
    </section>
  );
}

function SavedRow({ item }: { item: SavedItem }) {
  const [pending, start] = useTransition();
  const { applyServerCart } = useCartStore();
  return (
    <li className="flex items-center gap-4 py-3" aria-busy={pending}>
      {item.imageUrl ? (
        // eslint-disable-next-line @next/next/no-img-element
        <img
          src={mediaThumb(item.imageUrl, 96)}
          alt={displayName(item.name, item.sku)}
          className="size-12 shrink-0 rounded-lg border border-border object-cover"
        loading="lazy" decoding="async" />
      ) : (
        <div className="flex size-12 shrink-0 items-center justify-center rounded-lg border border-border bg-muted text-[9px] text-muted-foreground">
          No image
        </div>
      )}
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium text-foreground">
          {displayName(item.name, item.sku)}
        </p>
        <p className="text-xs text-muted-foreground">
          {item.sku}
          {item.quantity > 1 ? ` · qty ${item.quantity}` : ""}
        </p>
      </div>
      <button
        type="button"
        disabled={pending}
        onClick={() =>
          start(async () => {
            const cart = await moveSavedToCartAction(item.id);
            applyServerCart(cart);
          })
        }
        className="rounded-md border border-border px-3 py-1.5 text-xs font-semibold transition-colors hover:bg-muted disabled:opacity-50"
      >
        {pending ? "Moving…" : "Move to cart"}
      </button>
      <button
        type="button"
        disabled={pending}
        onClick={() =>
          start(async () => {
            const cart = await removeSavedItemAction(item.id);
            applyServerCart(cart);
          })
        }
        className="text-xs text-muted-foreground transition-colors hover:text-destructive disabled:opacity-50"
      >
        Remove
      </button>
    </li>
  );
}
