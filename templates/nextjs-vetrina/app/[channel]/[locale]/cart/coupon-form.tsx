"use client";

import { Link } from "@/app/components/LocaleLink";
import { useEffect, useTransition } from "react";
import { useTranslations } from "next-intl";

import { FieldErrors } from "@/app/components/account/AccountUI";
import { applyCoupon, removeCouponAction } from "@skelet/storefront-next/cart/actions";
import { couponSchema } from "@skelet/storefront-core/cart/schemas";
import { useActionForm } from "@skelet/storefront-react/forms/use-action-form";
import { useCartStore } from "@skelet/storefront-next/cart/store";
import type { AppliedPromotionCode } from "@skelet/storefront-core/cart/types";
import { formatPrice } from "@skelet/storefront-core/format";

// Three promotion shapes: typed codes (removable), fired auto-rules whose free gifts aren't in the cart yet (a nudge, not a discount), and
// everything else (applied, no Remove). Removal is fire-and-forget - there's no failure the customer could act on
export function CouponForm({
  appliedCodes,
  totalDiscount,
  currency,
}: {
  appliedCodes: AppliedPromotionCode[];
  totalDiscount: number;
  currency: string | { code: string; symbol?: string; symbolPosition?: string };
}) {
  const { applyServerCart } = useCartStore();
  const t = useTranslations("cart.coupon");
  const {
    form,
    fields,
    formAction,
    onSubmit,
    pending,
    lastResult,
  } = useActionForm(applyCoupon, couponSchema);
  const code = fields.code;

  // Push a successful apply's returned cart into the store so the summary + line list
  // reflect the new discount instantly (no revalidate)
  useEffect(() => {
    if (lastResult?.cart !== undefined) applyServerCart(lastResult.cart);
  }, [lastResult, applyServerCart]);

  const buckets = bucketize(appliedCodes);

  return (
    <section className="glass mt-8 rounded-xl p-5">
      <h2 className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
        {t("title")}
      </h2>

      <form
        id={form.id}
        onSubmit={onSubmit}
        action={formAction}
        noValidate
        className="mt-3 flex gap-2"
      >
        <input
          type="text"
          key={code.key}
          name={code.name}
          defaultValue={code.initialValue ?? ""}
          placeholder={t("placeholder")}
          aria-label={t("placeholder")}
          aria-invalid={code.errors ? true : undefined}
          aria-describedby={code.errors ? code.errorId : undefined}
          autoComplete="off"
          className="flex-1 rounded-lg border border-input bg-card px-3 py-2 text-sm tracking-widest uppercase outline-none placeholder:normal-case placeholder:tracking-normal focus:border-ring"
        />
        <button
          type="submit"
          disabled={pending}
          className="rounded-md bg-primary px-5 py-2 text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
        >
          {pending ? t("applying") : t("apply")}
        </button>
      </form>

      {code.errors ? (
        <p className="mt-2">
          <FieldErrors id={code.errorId} errors={code.errors} />
        </p>
      ) : null}

      {/* Rule fired, gift not in cart yet - above the applied list so the offer gets noticed */}
      {buckets.nudges.length > 0 ? (
        <ul className="mt-4 space-y-2">
          {buckets.nudges.map(({ code, gift }) => (
            <NudgeTile
              key={`${code.ruleName}-${gift.sku}`}
              code={code}
              gift={gift}
            />
          ))}
        </ul>
      ) : null}

      {buckets.active.length > 0 ? (
        <ul className="mt-4 space-y-2">
          {buckets.active.map((c) => (
            <AppliedTile key={c.code || c.ruleName} code={c} currency={currency} />
          ))}
        </ul>
      ) : null}

      {totalDiscount > 0 ? (
        <div className="mt-4 flex items-center justify-between text-sm">
          <span className="text-muted-foreground">{t("totalDiscount")}</span>
          <span className="font-medium tabular-nums text-emerald-700">
            −{formatPrice(totalDiscount, currency)}
          </span>
        </div>
      ) : null}
    </section>
  );
}
function bucketize(codes: AppliedPromotionCode[]): {
  nudges: { code: AppliedPromotionCode; gift: AppliedPromotionCode["freeGifts"][number] }[];
  active: AppliedPromotionCode[];
} {
  const nudges: ReturnType<typeof bucketize>["nudges"] = [];
  const active: AppliedPromotionCode[] = [];
  for (const c of codes) {
    if (c.invalidatedReason !== "") {
      active.push(c);
      continue;
    }
    const pending = c.freeGifts.filter((g) => !g.inCart);
    const hasOtherEffect =
      c.subtotalDiscount > 0 ||
      c.freeShipping ||
      c.freeGifts.some((g) => g.inCart);
    if (pending.length === 0) {
      active.push(c);
      continue;
    }
    for (const gift of pending) {
      nudges.push({ code: c, gift });
    }
    if (hasOtherEffect) {
      active.push(c);
    }
  }
  return { nudges, active };
}

function NudgeTile({
  code,
  gift,
}: {
  code: AppliedPromotionCode;
  gift: AppliedPromotionCode["freeGifts"][number];
}) {
  const t = useTranslations("cart.coupon");
  return (
    <li className="rounded-lg border border-sky-300 bg-sky-50 px-3 py-2 text-sm">
      <p className="font-medium text-sky-900">
        {code.publicLabel || code.ruleName}
      </p>
      <p className="mt-0.5 text-xs text-sky-800">
        {gift.quantity > 1 ? `${gift.quantity}× ` : ""}
        {t.rich("giftNudge", {
          sku: () => <span className="font-mono">{gift.sku}</span>,
          add: (chunks) =>
            gift.href ? (
              <Link
                href={gift.href}
                className="font-medium underline underline-offset-2 hover:text-sky-700"
              >
                {chunks}
              </Link>
            ) : (
              chunks
            ),
        })}
      </p>
    </li>
  );
}

function AppliedTile({
  code: c,
  currency,
}: {
  code: AppliedPromotionCode;
  currency: string | { code: string; symbol?: string; symbolPosition?: string };
}) {
  const { applyServerCart } = useCartStore();
  const t = useTranslations("cart.coupon");
  const tc = useTranslations("common");
  const [removing, startRemove] = useTransition();
  const invalidated = c.invalidatedReason !== "";
  const giftsInCart = c.freeGifts.filter((g) => g.inCart);
  return (
    <li
      className={`flex items-center justify-between gap-3 rounded-lg border px-3 py-2 text-sm ${
        invalidated
          ? "border-amber-300 bg-amber-50"
          : "border-emerald-300 bg-emerald-50"
      }`}
    >
      <div className="min-w-0">
        {/* Auto rules have no code, so they show the rule's public label instead */}
        {c.code ? (
          <p className="font-mono text-xs font-semibold uppercase tracking-wider">
            {c.code}
          </p>
        ) : (
          <p className="text-xs font-semibold">
            {c.publicLabel || c.ruleName}
          </p>
        )}
        {c.code && (c.publicLabel || c.ruleName) ? (
          <p className="truncate text-xs text-muted-foreground">
            {c.publicLabel || c.ruleName}
          </p>
        ) : null}
        {invalidated ? (
          <p className="mt-0.5 text-xs text-amber-700">
            {t("noLongerValid", { reason: c.invalidatedReason })}
          </p>
        ) : null}
        {giftsInCart.length > 0 && !invalidated ? (
          <p className="mt-0.5 text-xs text-emerald-700">
            {t("giftsApplied", {
              count: giftsInCart.length,
              items: giftsInCart
                .map((g) => `${g.quantity > 1 ? `${g.quantity}× ` : ""}${g.sku}`)
                .join(", "),
            })}
          </p>
        ) : null}
      </div>
      <div className="flex shrink-0 items-center gap-3">
        {!invalidated && c.subtotalDiscount > 0 ? (
          <span className="text-sm font-medium tabular-nums">
            −{formatPrice(c.subtotalDiscount, currency)}
          </span>
        ) : null}
        {!invalidated && c.freeShipping ? (
          <span className="rounded-sm bg-emerald-200 px-2 py-0.5 text-[10px] font-medium uppercase tracking-wider text-emerald-900">
            {t("freeShip")}
          </span>
        ) : null}
        {/* Auto rules can't be removed - hide the button rather than offer a no-op */}
        {c.requiresCode ? (
          <button
            type="button"
            disabled={removing}
            onClick={() =>
              startRemove(async () => {
                const cart = await removeCouponAction(c.code);
                applyServerCart(cart);
              })
            }
            className="text-xs text-muted-foreground transition-colors hover:text-destructive disabled:opacity-50"
            aria-label={t("remove", { code: c.code })}
          >
            {removing ? tc("removing") : tc("remove")}
          </button>
        ) : null}
      </div>
    </li>
  );
}
