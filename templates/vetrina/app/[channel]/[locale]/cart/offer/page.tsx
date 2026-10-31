import type { Metadata } from "next";
import { getFormatter, getTranslations } from "next-intl/server";
import { redirect } from "next/navigation";

import { Link } from "@/app/components/LocaleLink";
import { MarkOfferOpened } from "@/app/components/offer/MarkOfferOpened";
import { getAccessToken } from "@skelet/storefront-next/auth/session";
import { continueOfferCheckout, removeOfferLine, updateOfferLine } from "@skelet/storefront-next/cart/offer-actions";
import { getOfferSession, readOffer } from "@skelet/storefront-next/cart/offer";
import { withLocalePrefix } from "@skelet/storefront-next/locale/path";
import { offerLocks, type CartOfferLock } from "@skelet/storefront-next/cart/types";
import { getChannel } from "@skelet/storefront-next/channel/read";
import { displayName } from "@skelet/storefront-next/display";
import { formatPrice } from "@skelet/storefront-next/format";
import { mediaThumb } from "@skelet/storefront-next/media-thumb";

export async function generateMetadata({ params }: PageProps<"/[channel]/[locale]/cart/offer">): Promise<Metadata> {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "offer" });
  // The link carries a credential in its landing URL, and the page is the shopper's alone
  return { title: t("title"), robots: { index: false, follow: false } };
}

const STATES = ["invalid", "expired", "completed", "revoked", "other-account"] as const;
type OfferPageState = (typeof STATES)[number];
const LOCK_ORDER: CartOfferLock[] = ["LINES", "SHIPPING", "BILLING", "DELIVERY", "COUPONS"];

// The cart the store prepared for the shopper - reached through its emailed link (app/offer/[token]) or the
// account's Offers list. It sits beside the shopper's own cart; Continue moves checkout onto it
export default async function OfferPage({ params, searchParams }: PageProps<"/[channel]/[locale]/cart/offer">) {
  const { locale } = await params;
  const { state: requested, cartId: requestedCart } = await searchParams;
  const cartId = typeof requestedCart === "string" ? requestedCart : undefined;
  const t = await getTranslations({ locale, namespace: "offer" });
  const format = await getFormatter({ locale });
  const [{ cart, state: unreadable }, channel, session, signedIn] = await Promise.all([
    readOffer(cartId),
    getChannel(),
    getOfferSession(),
    getAccessToken().then(Boolean),
  ]);
  const currency = channel?.currency ?? "USD";

  // A held link to an account's offer opens once its owner signs in
  if (unreadable === "invalid" && session?.token && !signedIn) {
    redirect(await withLocalePrefix(`/account/login?returnTo=${encodeURIComponent(cartId ? `/cart/offer?cartId=${encodeURIComponent(cartId)}` : "/cart/offer")}`, locale));
  }
  const explained = STATES.find((s) => s === requested);
  const state: OfferPageState | null = explained ?? unreadable ?? (cart?.offer ? stateOf(cart.offer.state) : "invalid");
  if (state || !cart?.offer) {
    const s = state ?? "invalid";
    return (
      <main className="mx-auto w-full max-w-2xl px-6 pt-10 pb-16">
        <h1 className="text-2xl font-extrabold tracking-tight">{t(`states.${s}.title`)}</h1>
        <p className="mt-3 text-muted-foreground">{t(`states.${s}.body`)}</p>
        <Link href="/" className="mt-6 inline-block text-sm underline underline-offset-2">
          {t("backToCart")}
        </Link>
      </main>
    );
  }

  const offer = cart.offer;
  const linesLocked = offerLocks(cart, "LINES");
  const locked = LOCK_ORDER.filter((l) => offer.locks.includes(l)).map((l) => t(`lock.${l}`));

  return (
    <main className="mx-auto w-full max-w-3xl px-6 pt-6 pb-12">
      {offer.state === "DRAFT" || offer.state === "SENT" ? <MarkOfferOpened cartId={cart.id} /> : null}
      <h1 className="text-3xl font-extrabold tracking-tight">{t("title")}</h1>
      <p className="mt-2 text-sm text-muted-foreground">
        {t("expires", { date: format.dateTime(new Date(offer.expiresAt), { dateStyle: "long", timeStyle: "short" }) })}
      </p>
      {offer.message ? <p className="mt-4 whitespace-pre-line rounded-xl border border-border p-4 text-sm">{offer.message}</p> : null}
      {locked.length > 0 ? <p className="mt-4 text-xs text-muted-foreground">{t("locked", { parts: locked.join(", ") })}</p> : null}

      <ul className="mt-6 divide-y divide-border border-y border-border">
        {cart.lines.map((line) => (
          <li key={line.id} className="flex items-start gap-4 py-4">
            {line.imageUrl ? (
              // eslint-disable-next-line @next/next/no-img-element
              <img
                src={mediaThumb(line.imageUrl, 128)}
                alt={displayName(line.name, line.sku)}
                className="size-16 shrink-0 rounded-lg border border-border object-cover"
                loading="lazy"
                decoding="async"
              />
            ) : (
              <div className="size-16 shrink-0 rounded-lg border border-border bg-muted" />
            )}
            <div className="min-w-0 flex-1">
              <p className="text-sm font-medium">
                {displayName(line.name, line.sku)}
                {line.isCustom ? <span className="ml-2 text-[10px] uppercase tracking-wider text-muted-foreground">{t("custom")}</span> : null}
              </p>
              <p className="mt-1 text-xs text-muted-foreground">{t("each", { price: formatPrice(line.unitPrice, currency) })}</p>
              {line.options.length > 0 ? (
                <ul className="mt-1 text-xs text-muted-foreground">
                  {line.options.map((o) => (
                    <li key={o.id}>
                      {o.inputType === "rule" ? o.optionTitle : `${o.optionTitle}: ${o.valueLabel || o.textValue}`}
                    </li>
                  ))}
                </ul>
              ) : null}
              {linesLocked ? (
                <p className="mt-2 text-xs">× {line.quantity}</p>
              ) : (
                <div className="mt-2 flex items-center gap-3">
                  <form action={updateOfferLine} className="flex items-center gap-2">
                    <input type="hidden" name="lineId" value={line.id} />
                    <input
                      type="number"
                      name="quantity"
                      min={1}
                      step="any"
                      defaultValue={line.quantity}
                      aria-label={t("quantity")}
                      className="h-9 w-20 rounded-md border border-border bg-background px-2 text-sm tabular-nums"
                    />
                    <button type="submit" className="inline-flex h-9 items-center text-xs underline underline-offset-2">
                      {t("update")}
                    </button>
                  </form>
                  <form action={removeOfferLine} className="flex items-center">
                    <input type="hidden" name="lineId" value={line.id} />
                    <button type="submit" className="inline-flex h-9 items-center text-xs text-muted-foreground underline underline-offset-2">
                      {t("remove")}
                    </button>
                  </form>
                </div>
              )}
            </div>
            <p className="text-sm tabular-nums">{formatPrice(line.totalPrice, currency)}</p>
          </li>
        ))}
      </ul>

      <div className="mt-6 text-right">
        <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground">{t("subtotal")}</p>
        <p className="text-2xl font-semibold tabular-nums">{formatPrice(Math.max(0, cart.subtotal - cart.totalDiscount), currency)}</p>
        <p className="mt-1 text-xs text-muted-foreground">{t("taxNote")}</p>
      </div>

      <form action={continueOfferCheckout} className="mt-8 flex justify-end">
        <input type="hidden" name="cartId" value={cart.id} />
        <button
          type="submit"
          disabled={cart.lines.length === 0}
          className="inline-flex h-12 items-center justify-center rounded-md bg-primary px-8 text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
        >
          {t("continue")}
        </button>
      </form>
    </main>
  );
}

// An offer that can't be completed any more says why rather than showing a cart
function stateOf(s: string): OfferPageState | null {
  if (s === "EXPIRED") return "expired";
  if (s === "COMPLETED") return "completed";
  if (s === "REVOKED") return "revoked";
  return null;
}
