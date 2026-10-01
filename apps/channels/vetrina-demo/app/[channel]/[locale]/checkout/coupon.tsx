"use client";

import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { useEffect } from "react";

import { FieldErrors, inputClass } from "@/app/components/account/AccountUI";
import { applyCoupon } from "@/app/lib/cart/actions";
import { couponSchema } from "@/app/lib/cart/schemas";
import { useCartStore } from "@/app/lib/cart/store";
import { useActionForm } from "@/app/lib/forms/use-action-form";

// A discount code at checkout, collapsed behind a link - an open box sends shoppers off hunting for codes. The cart
// page's form lists and removes codes; here the summary already lists them, so this only adds one and re-reads the page,
// since checkout's totals are priced server-side
export function CheckoutCoupon() {
  const t = useTranslations("cart.coupon");
  const tc = useTranslations("checkout");
  const router = useRouter();
  const { applyServerCart } = useCartStore();
  const { form, fields, formAction, onSubmit, pending, lastResult } = useActionForm(applyCoupon, couponSchema);
  const code = fields.code;

  useEffect(() => {
    if (lastResult?.cart === undefined) return;
    applyServerCart(lastResult.cart);
    router.refresh();
  }, [lastResult, applyServerCart, router]);

  return (
    <details className="mt-4 border-t border-border pt-3 text-sm" open={!!code.errors}>
      <summary className="cursor-pointer text-xs underline underline-offset-2">{tc("discountCode")}</summary>
      <form id={form.id} onSubmit={onSubmit} action={formAction} noValidate className="mt-2 grid gap-1">
        <div className="flex gap-2">
          <input
            type="text"
            key={code.key}
            name={code.name}
            defaultValue={typeof code.initialValue === "string" ? code.initialValue : ""}
            aria-label={tc("discountCode")}
            aria-invalid={code.errors ? true : undefined}
            aria-describedby={code.errors ? code.errorId : undefined}
            autoComplete="off"
            className={`${inputClass} min-w-0 flex-1 font-mono`}
          />
          <button
            type="submit"
            disabled={pending}
            className="rounded-md border border-border px-3 text-sm font-semibold transition-colors hover:bg-muted disabled:opacity-50"
          >
            {pending ? t("applying") : t("apply")}
          </button>
        </div>
        {code.errors ? <FieldErrors id={code.errorId} errors={code.errors} /> : null}
      </form>
    </details>
  );
}
