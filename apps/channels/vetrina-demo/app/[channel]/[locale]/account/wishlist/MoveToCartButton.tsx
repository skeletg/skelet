"use client";

import { useState, useTransition } from "react";
import { useTranslations } from "next-intl";

import { useCartUI } from "@/app/components/CartUIProvider";
import { moveToCartAction } from "@/app/lib/wishlist/actions";

export function MoveToCartButton({ productId }: { productId: string }) {
  const t = useTranslations("account.wishlist");
  const [pending, startTransition] = useTransition();
  const [error, setError] = useState<string | null>(null);
  const { open } = useCartUI();

  return (
    <div>
      <button
        type="button"
        disabled={pending}
        onClick={() =>
          startTransition(async () => {
            setError(null);
            const res = await moveToCartAction(productId);
            if (res.ok) open();
            else setError(res.error ?? t("moveFailed"));
          })
        }
        className="mt-2 w-full rounded-md border border-border bg-background py-2 text-xs font-semibold transition-colors hover:bg-accent disabled:opacity-60"
      >
        {pending ? t("moving") : t("moveToCart")}
      </button>
      {error && (
        <p role="alert" className="mt-1 text-xs text-red-600">
          {error}
        </p>
      )}
    </div>
  );
}
