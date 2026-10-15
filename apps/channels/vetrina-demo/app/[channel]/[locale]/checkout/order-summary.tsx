import { getTranslations } from "next-intl/server";

import { ApproxPrice } from "@/app/components/ApproxPrice";
import { CartLineOptionsList } from "@/app/components/CartLineOptionsList";
import { ChargeCurrencyNote } from "@/app/components/ChargeCurrencyNote";
import { Link } from "@/app/components/LocaleLink";
import { formatPrice } from "@/app/lib/cart/read";
import type { Cart } from "@/app/lib/cart/types";
import type { CheckoutState } from "@/app/lib/checkout/read";
import { displayName } from "@/app/lib/display";
import { mediaThumb } from "@/app/lib/media-thumb";
import { describeAppliedCode, giftSummary } from "@/app/lib/promotions/applied-code";

import { CheckoutCoupon } from "./coupon";

type Currency = string | { code: string; symbol?: string; symbolPosition?: string };

// The cart as checkout will charge it - lines with their picks, applied codes, a discount code field and the totals the
// placement path prices. Shipping and tax show once there's a destination to price them for
export async function OrderSummary({
  cart,
  checkout,
  currency,
  locale,
}: {
  cart: Cart;
  checkout: CheckoutState;
  currency: Currency;
  locale: string;
}) {
  const t = await getTranslations({ locale, namespace: "checkout" });
  const to = await getTranslations({ locale, namespace: "order" });
  const priced = checkout.destination !== null;
  return (
    <>
      <ul className="space-y-3">
        {cart.lines.map((line) => (
          <li key={line.id} className="flex items-start gap-3">
            <span className="relative shrink-0">
              {line.imageUrl ? (
                // eslint-disable-next-line @next/next/no-img-element
                <img
                  src={mediaThumb(line.imageUrl, 96)}
                  alt=""
                  width={48}
                  height={48}
                  className="size-12 rounded-md border border-border object-cover"
                  loading="lazy"
                  decoding="async"
                />
              ) : (
                <span className="block size-12 rounded-md border border-border bg-muted" aria-hidden />
              )}
              <span className="absolute -right-1.5 -top-1.5 grid min-w-5 place-items-center rounded-full bg-foreground px-1 text-[11px] font-semibold text-background">
                {line.quantity}
              </span>
            </span>
            <div className="min-w-0 flex-1">
              <p className="truncate">{displayName(line.name, line.sku)}</p>
              {/* Same pick summary the cart line shows - the last look before payment shouldn't hide what was configured */}
              <CartLineOptionsList options={line.options} className="mt-0.5 space-y-0.5 text-xs text-muted-foreground" />
            </div>
            <span className="shrink-0 tabular-nums">{formatPrice(line.totalPrice, currency)}</span>
          </li>
        ))}
      </ul>

      {cart.appliedCodes.length > 0 ? (
        <ul className="mt-4 space-y-1 border-t border-border pt-3 text-xs">
          {cart.appliedCodes.map((c) => {
            const b = describeAppliedCode(c);
            return (
              <li
                key={c.code || b.label}
                className={`flex justify-between gap-2 ${b.invalidatedReason ? "text-amber-700" : "text-emerald-700"}`}
              >
                <span className="min-w-0 truncate">
                  <span className={c.code ? "font-mono" : ""}>{b.label}</span>
                  {b.invalidatedReason ? (
                    <span className="text-muted-foreground">{t("noLongerValid", { reason: b.invalidatedReason })}</span>
                  ) : null}
                  {b.giftsInCart.length > 0 ? (
                    <span>{t("freeGifts", { gifts: giftSummary(b.giftsInCart) })}</span>
                  ) : null}
                </span>
                {/* Only a real subtotal discount gets a money line - a free-shipping or gift code has none, and printing
                    -$0.00 reads as broken */}
                {b.discountCents > 0 ? (
                  <span className="shrink-0 tabular-nums">−{formatPrice(b.discountCents, currency)}</span>
                ) : b.freeShipping ? (
                  <span className="shrink-0 uppercase tracking-wider">{t("freeShip")}</span>
                ) : null}
              </li>
            );
          })}
        </ul>
      ) : null}

      <CheckoutCoupon />

      <Totals
        totals={checkout.totals}
        priced={priced}
        currency={currency}
        labels={{
          subtotal: to("subtotal"),
          discount: to("discount"),
          shipping: to("shipping"),
          tax: to("tax"),
          total: to("total"),
          afterPostcode: t("afterPostcode"),
          giftCards: t("giftCardsApplied"),
          storeCredit: t("storeCreditApplied"),
          amountDue: t("amountDue"),
        }}
      />

      <Link href="/cart" className="mt-6 inline-block text-xs text-muted-foreground underline">
        {t("editCart")}
      </Link>
    </>
  );
}

// The cart's own totals, priced the way placement charges - shipping and tax only once there's a destination
function Totals({
  totals,
  priced,
  currency,
  labels,
}: {
  totals: CheckoutState["totals"];
  priced: boolean;
  currency: Currency;
  labels: Record<
    "subtotal" | "discount" | "shipping" | "tax" | "total" | "afterPostcode" | "giftCards" | "storeCredit" | "amountDue",
    string
  >;
}) {
  const tenders = totals.giftCardCents + totals.storeCreditCents;
  return (
    <>
      <dl className="mt-4 space-y-1 border-t border-border pt-3 text-sm" aria-live="polite">
        <Row label={labels.subtotal}>
          {formatPrice(totals.subtotalCents, currency)}
          <ApproxPrice cents={totals.subtotalCents} />
        </Row>
        {totals.discountCents > 0 ? (
          <Row label={labels.discount} className="text-emerald-700">
            −{formatPrice(totals.discountCents, currency)}
          </Row>
        ) : null}
        <Row label={labels.shipping}>
          {priced ? (
            formatPrice(totals.shippingCents, currency)
          ) : (
            <span className="text-muted-foreground">{labels.afterPostcode}</span>
          )}
        </Row>
        <Row label={labels.tax}>
          {priced ? (
            formatPrice(totals.taxCents, currency)
          ) : (
            <span className="text-muted-foreground">{labels.afterPostcode}</span>
          )}
        </Row>
        {totals.giftCardCents > 0 ? (
          <Row label={labels.giftCards}>−{formatPrice(totals.giftCardCents, currency)}</Row>
        ) : null}
        {totals.storeCreditCents > 0 ? (
          <Row label={labels.storeCredit}>−{formatPrice(totals.storeCreditCents, currency)}</Row>
        ) : null}
      </dl>
      <ChargeCurrencyNote />
      <p className="mt-4 flex justify-between border-t border-border pt-3 text-base font-semibold">
        <span>{tenders > 0 ? labels.amountDue : labels.total}</span>
        <span className="tabular-nums">
          {formatPrice(tenders > 0 ? totals.amountDueCents : totals.totalCents, currency)}
        </span>
      </p>
    </>
  );
}

function Row({ label, children, className }: { label: string; children: React.ReactNode; className?: string }) {
  return (
    <div className={`flex justify-between gap-2 ${className ?? ""}`.trim()}>
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="tabular-nums">{children}</dd>
    </div>
  );
}
