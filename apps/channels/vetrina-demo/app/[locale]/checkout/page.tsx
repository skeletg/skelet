import { redirect } from "next/navigation";

import { withLocalePrefix } from "@/app/lib/locale/path";
import { Link } from "@/app/components/LocaleLink";

import { AnalyticsEvent } from "@/app/components/AnalyticsEvent";
import { CartLineOptionsList } from "@/app/components/CartLineOptionsList";
import { getCart, formatPrice } from "@/app/lib/cart/read";
import { ApproxPrice } from "@/app/components/ApproxPrice";
import { ChargeCurrencyNote } from "@/app/components/ChargeCurrencyNote";
import { getResolvedConsent } from "@/app/lib/consent/read";
import { getCountries, getCountrySettings, getRegionsByCountry } from "@/app/lib/countries/read";
import { getGuestCheckoutAllowed } from "@/app/lib/config/read";
import { displayName } from "@/app/lib/display";
import {
  describeAppliedCode,
  giftSummary,
} from "@/app/lib/promotions/applied-code";
import { getCurrentCustomer } from "@/app/lib/customer/read";
import { getMyPaymentMethods } from "@/app/lib/payment-methods/actions";
import { getChannel } from "@/app/lib/channel/read";
import { gqlFetch } from "@/app/lib/graphql";
import { PickupLocationsDocument, ShippingQuotesDocument } from "@/app/lib/checkout/operations.gql";

import { CheckoutForm } from "./CheckoutForm";

export const metadata = { title: "Checkout" };

export default async function CheckoutPage({ params }: PageProps<"/[locale]/checkout">) {
  // Locale straight off the segment - a page taking no props has no request scope for
  // getLocale() to read, so it silently answered with the default language
  const { locale } = await params;

  const [customer, cart, channel, consent, countries, countrySettings, guestAllowed] =
    await Promise.all([
      getCurrentCustomer(),
      getCart(),
      getChannel(),
      getResolvedConsent(),
      getCountries(),
      getCountrySettings(),
      getGuestCheckoutAllowed(),
    ]);

  if (!cart || cart.lines.length === 0) {
    redirect(await withLocalePrefix("/cart", locale));
  }

  // Seeded ISO 3166-2 subdivisions for the offered countries → the address forms render a state dropdown where we have
  // data, free text elsewhere. Server-side; one query per country (most return empty)
  const regionsByCountry = await getRegionsByCountry(countries.map((c) => c.code));

  const currency = channel?.currency ?? "USD";

  const initialCountry =
    customer?.defaultShippingAddress?.countryCode ??
    customer?.addresses[0]?.countryCode ??
    "";
  let initialShippingQuotes: Array<{
    carrierSlug: string;
    carrierName: string;
    code: string;
    name: string;
    priceCents: number;
  }> = [];
  if (initialCountry) {
    try {
      const res = await gqlFetch(ShippingQuotesDocument, {
        countryCode: initialCountry,
      });
      initialShippingQuotes = res.data.shippingQuotes;
    } catch {
      // Country not covered by any zone - UI shows "no shipping method available for
      // this address"
    }
  }

  let pickupLocations: Array<{
    slug: string;
    name: string;
    line1: string;
    city: string;
    postalCode: string;
    countryCode: string;
    instructions: string;
  }> = [];
  try {
    const res = await gqlFetch(PickupLocationsDocument);
    pickupLocations = res.data.pickupLocations;
  } catch {
    // Click-and-collect off / unreachable - the pickup group just doesn't render and
    // delivery options are unaffected
  }

  const subtotal = cart.subtotal;
  const discount = cart.totalDiscount;
  const freeShipping = cart.appliedCodes.some((c) => c.freeShipping);
  const cheapestShip = freeShipping
    ? 0
    : initialShippingQuotes.length === 0
      ? null
      : Math.min(...initialShippingQuotes.map((q) => q.priceCents));
  const previewTotal =
    Math.max(0, subtotal - discount) + (cheapestShip ?? 0);

  return (
    <main className="mx-auto grid w-full max-w-8xl gap-10 px-6 md:px-10 pt-6 pb-12 md:grid-cols-[1fr_22rem]">
      <AnalyticsEvent
        enabled={consent.marketing}
        event="begin_checkout"
        payload={{
          ecommerce: {
            currency:
              typeof currency === "string" ? currency : currency.code,
            value: previewTotal / 100,
            items: cart.lines.map((line) => ({
              item_id: line.productSku,
              item_name: displayName(line.productName, line.productSku),
              quantity: line.quantity,
              price: line.unitPrice / 100,
            })),
          },
        }}
      />
      <div>
        <h1 className="text-3xl font-extrabold tracking-tight">Checkout</h1>
        {!customer && !guestAllowed ? (
          // The backend refuses guest placement outright when this is off (GUEST_CHECKOUT_DISABLED) -
          // saying so up front beats letting someone fill the whole form and get rejected at submit
          <div className="mt-6 border border-border p-6">
            <p className="text-sm font-medium">This store requires an account to check out.</p>
            <p className="mt-1 text-sm text-muted-foreground">
              Sign in or create an account to place your order. Your cart is saved.
            </p>
            <div className="mt-4 flex gap-3">
              <a
                href={`/account/login?returnTo=${encodeURIComponent("/checkout")}`}
                className="inline-flex h-11 items-center justify-center bg-foreground px-6 font-mono text-[13px] uppercase tracking-[0.14em] text-background"
              >
                Sign in
              </a>
              <a
                href={`/account/register?returnTo=${encodeURIComponent("/checkout")}`}
                className="inline-flex h-11 items-center justify-center border border-foreground px-6 font-mono text-[13px] uppercase tracking-[0.14em]"
              >
                Create account
              </a>
            </div>
          </div>
        ) : customer ? (
          <p className="mt-2 text-sm text-muted-foreground">
            Signed in as {customer.email}.
          </p>
        ) : (
          <p className="mt-2 text-sm text-muted-foreground">
            Checking out as guest.{" "}
            <a
              href={`/account/login?returnTo=${encodeURIComponent("/checkout")}`}
              className="underline underline-offset-2"
            >
              Sign in
            </a>{" "}
            instead to use a saved address.
          </p>
        )}

        {/* Gated: the form is not rendered at all, so nobody fills in a checkout the API will refuse */}
        {(customer || guestAllowed) && (
        <CheckoutForm
          customerEmail={customer?.email ?? ""}
          storeCreditBalance={customer?.storeCreditBalance ?? 0}
          savedPaymentMethods={customer ? await getMyPaymentMethods() : []}
          addresses={customer?.addresses ?? []}
          defaultBillingId={customer?.defaultBillingAddress?.id ?? null}
          defaultShippingId={customer?.defaultShippingAddress?.id ?? null}
          initialShippingQuotes={initialShippingQuotes}
          pickupLocations={pickupLocations}
          countries={countries}
          regionsByCountry={regionsByCountry}
          countrySettings={countrySettings}
          currency={currency}
          initialNote={cart?.note ?? ""}
        />
        )}
      </div>

      <aside className="glass self-start rounded-xl p-6 text-sm md:sticky md:top-24">
        <h2 className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
          Order summary
        </h2>
        <ul className="mt-4 space-y-2">
          {cart.lines.map((line) => (
            <li key={line.id} className="flex justify-between gap-3">
              <div className="min-w-0">
                <p className="truncate">
                  {displayName(line.productName, line.productSku)}
                  <span className="text-muted-foreground"> × {line.quantity}</span>
                </p>
                {/* Same pick summary the cart line shows - the last look before payment
                    shouldn't hide what was configured */}
                <CartLineOptionsList
                  options={line.options}
                  className="mt-0.5 space-y-0.5 text-xs text-muted-foreground"
                />
              </div>
              <span className="shrink-0 tabular-nums">
                {formatPrice(line.totalPrice, currency)}
              </span>
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
                  className={`flex justify-between gap-2 ${
                    b.invalidatedReason ? "text-amber-700" : "text-emerald-700"
                  }`}
                >
                  <span className="min-w-0 truncate">
                    <span className={c.code ? "font-mono" : ""}>{b.label}</span>
                    {b.invalidatedReason ? (
                      <span className="text-muted-foreground">
                        {" "}
                        - no longer valid: {b.invalidatedReason}
                      </span>
                    ) : null}
                    {b.giftsInCart.length > 0 ? (
                      <span> - free {giftSummary(b.giftsInCart)}</span>
                    ) : null}
                  </span>
                  {/* Only a real subtotal discount gets a money line - a free-shipping
                      or gift code has none, and printing -$0.00 reads as broken */}
                  {b.discountCents > 0 ? (
                    <span className="shrink-0 tabular-nums">
                      −{formatPrice(b.discountCents, currency)}
                    </span>
                  ) : b.freeShipping ? (
                    <span className="shrink-0 uppercase tracking-wider">
                      Free ship
                    </span>
                  ) : null}
                </li>
              );
            })}
          </ul>
        ) : null}

        <dl className="mt-4 space-y-1 border-t border-border pt-3 text-sm">
          <Row label="Subtotal">
            {formatPrice(subtotal, currency)}
            <ApproxPrice cents={subtotal} />
          </Row>
          {discount > 0 ? (
            <Row label="Discount" className="text-emerald-700">
              −{formatPrice(discount, currency)}
            </Row>
          ) : null}
          {cheapestShip != null ? (
            <Row label={freeShipping ? "Shipping (free)" : "Shipping (est.)"}>
              {formatPrice(cheapestShip, currency)}
            </Row>
          ) : (
            <Row label="Shipping">
              <span className="text-muted-foreground">
                Not available for this address
              </span>
            </Row>
          )}
          <Row label="Tax">
            <span className="text-muted-foreground">
              Calculated at checkout
            </span>
          </Row>
        </dl>
        <ChargeCurrencyNote />
        <p className="mt-4 flex justify-between border-t border-border pt-3 text-base font-semibold">
          <span>Total</span>
          <span className="tabular-nums">
            {formatPrice(previewTotal, currency)}
          </span>
        </p>

        <Link
          href="/cart"
          className="mt-6 inline-block text-xs text-muted-foreground underline"
        >
          ← Edit cart
        </Link>
      </aside>
    </main>
  );
}

function Row({
  label,
  children,
  className,
}: {
  label: string;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <div
      className={`flex justify-between gap-2 ${className ?? ""}`.trim()}
    >
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="tabular-nums">{children}</dd>
    </div>
  );
}
