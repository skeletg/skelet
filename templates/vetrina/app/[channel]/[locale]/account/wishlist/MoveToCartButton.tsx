"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";

import { useErrorText } from "@/app/components/account/AccountUI";
import { useCartUI } from "@/app/components/CartUIProvider";
import { removeLocalWishlistItem } from "@/app/components/WishlistButton";
import { useCartStore } from "@/app/lib/cart/store";
import { moveToCartAction } from "@/app/lib/wishlist/actions";

export function MoveToCartButton({ productId }: { productId: string }) {
  const t = useTranslations("account.wishlist");
  const [pending, startTransition] = useTransition();
  const [error, setError] = useState<string | null>(null);
  const { open } = useCartUI();
  const { applyServerCart } = useCartStore();
  const errorText = useErrorText();
  const router = useRouter();

  return (
    <div>
      <button
        type="button"
        disabled={pending}
        onClick={() =>
          startTransition(async () => {
            setError(null);
            const res = await moveToCartAction(productId);
            if (res.ok) {
              // Local entry first: the refreshed page's WishlistMerge would otherwise see it missing server-side and re-add it
              removeLocalWishlistItem(productId);
              if (res.cart !== undefined) applyServerCart(res.cart);
              router.refresh();
              open();
            } else setError(res.error ?? "moveFailed");
          })
        }
        className="mt-2 w-full rounded-md border border-border bg-background py-2 text-xs font-semibold transition-colors hover:bg-accent disabled:opacity-60"
      >
        {pending ? t("moving") : t("moveToCart")}
      </button>
      {error && (
        <p role="alert" className="mt-1 text-xs text-red-600">
          {errorText(error)}
        </p>
      )}
    </div>
  );
}
