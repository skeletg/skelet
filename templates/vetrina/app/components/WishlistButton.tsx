"use client";

import { useCallback, useSyncExternalStore } from "react";
import { useTranslations } from "next-intl";
import { HeartIcon } from "lucide-react";

import { toggleWishlistAction } from "@skelet/storefront-next/wishlist/actions";

// Heart toggle for product cards + PDP. State strategy: localStorage is the instant source of truth for the FILLED
// state (works for guests and avoids per-card server fetches); signed-in customers also persist via the server action

const KEY = "storefront_wishlist";

export function readLocalWishlist(): string[] {
  try {
    const raw = localStorage.getItem(KEY);
    const v = raw ? (JSON.parse(raw) as unknown) : [];
    return Array.isArray(v) ? v.filter((x): x is string => typeof x === "string") : [];
  } catch {
    return [];
  }
}

// Subscribers so every heart for the same product repaints off one write - `storage` only
// fires in OTHER tabs, so local writes notify by hand
const listeners = new Set<() => void>();

function writeLocalWishlist(ids: string[]) {
  try {
    localStorage.setItem(KEY, JSON.stringify(ids.slice(0, 200)));
  } catch {
    // storage full/blocked - heart state just won't persist
  }
  listeners.forEach((fn) => fn());
}

// An item that left the wishlist some other way (moved to the cart) - left in the guest list, WishlistMerge would import it again
export function removeLocalWishlistItem(productId: string) {
  writeLocalWishlist(readLocalWishlist().filter((i) => i !== productId));
}

function subscribe(onChange: () => void) {
  listeners.add(onChange);
  window.addEventListener("storage", onChange);
  return () => {
    listeners.delete(onChange);
    window.removeEventListener("storage", onChange);
  };
}

export function WishlistButton({
  productId,
  className,
  size = "md",
}: {
  productId: string;
  className?: string;
  size?: "sm" | "md";
}) {
  // localStorage is client-only, so the server pass renders unsaved and the store takes over
  // on hydration - a boolean snapshot is value-compared, so no cached-getter dance needed
  const t = useTranslations("product");
  const saved = useSyncExternalStore(
    subscribe,
    useCallback(() => readLocalWishlist().includes(productId), [productId]),
    () => false,
  );

  function toggle(e: React.MouseEvent) {
    e.preventDefault();
    e.stopPropagation();
    const next = !saved;
    const ids = readLocalWishlist();
    writeLocalWishlist(next ? [productId, ...ids.filter((i) => i !== productId)] : ids.filter((i) => i !== productId));
    void toggleWishlistAction(productId, next);
  }

  return (
    <button
      type="button"
      aria-label={saved ? t("wishlistRemove") : t("wishlistSave")}
      aria-pressed={saved}
      onClick={toggle}
      className={
        "inline-flex items-center justify-center rounded-md border border-border/60 bg-background/80 backdrop-blur transition " +
        (size === "sm" ? "size-8" : "size-9") +
        (className ? " " + className : "")
      }
    >
      <HeartIcon
        className={
          (size === "sm" ? "size-4" : "size-4.5") +
          (saved ? " fill-red-500 text-red-500" : " text-muted-foreground")
        }
      />
    </button>
  );
}
