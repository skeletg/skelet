import type { Metadata } from "next";
import { getFormatter, getTranslations } from "next-intl/server";
import { redirect } from "next/navigation";

import { withLocalePrefix } from "@/app/lib/locale/path";

import { AnalyticsEvent } from "@/app/components/AnalyticsEvent";
import { getCart, formatPrice, getFreeShippingThreshold } from "@/app/lib/cart/read";
import {
  getCheckoutState,
  getPaymentMethods,
  getPickupLocations,
  getShippingQuotes,
  type CheckoutState,
} from "@/app/lib/checkout/read";
import { businessIdFieldsFor, companyCountry } from "@/app/lib/checkout/business-ids";
import { activeStep, detailsComplete, estimatedArrival, stepStatus, type CheckoutProgress } from "@/app/lib/checkout/step";
import { getResolvedConsent } from "@/app/lib/consent/read";
import { getCountries, getCountrySettings, getRegionsByCountry, getPhoneSettings } from "@/app/lib/countries/read";
import { getCheckoutSettings, getGuestCheckoutAllowed } from "@/app/lib/config/read";
import { displayName } from "@/app/lib/display";
import { getCurrentCustomer } from "@/app/lib/customer/read";
import { getMyPaymentMethods } from "@/app/lib/payment-methods/actions";
import { getChannel } from "@/app/lib/channel/read";

import { DeliveryStep, selectionOf } from "./delivery-step";
import { DetailsStep } from "./details-step";
import { OrderSummary } from "./order-summary";
import { PaymentStep } from "./payment-step";
import { oneLine, StepCard } from "./summary";

export async function generateMetadata({
  params,
}: PageProps<"/[channel]/[locale]/checkout">): Promise<Metadata> {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "checkout" });
  return { title: t("title") };
}

// Every step is saved on the cart, so this page is rebuilt from the cart on each visit and after each step - it opens
// the first step still owed, or an earlier one reopened with ?step=. Three steps, in the order IKEA and Baymard's
// research agree on: how the order arrives (from a postcode alone), who it's for, then payment
export default async function CheckoutPage({
  params,
  searchParams,
}: PageProps<"/[channel]/[locale]/checkout">) {
  // Locale straight off the segment - a page taking no props has no request scope for
  // getLocale() to read, so it silently answered with the default language
  const { locale } = await params;
  const { step: requestedStep, notice } = await searchParams;
  const t = await getTranslations({ locale, namespace: "checkout" });
  const tc = await getTranslations({ locale, namespace: "common" });
  const format = await getFormatter({ locale });

  const [customer, cart, channel, consent, countries, countrySettings, guestAllowed, settings, freeShipThreshold, phoneSettings] =
    await Promise.all([
      getCurrentCustomer(),
      getCart(),
      getChannel(),
      getResolvedConsent(),
      getCountries(),
      getCountrySettings(),
      getGuestCheckoutAllowed(),
      getCheckoutSettings(),
      getFreeShippingThreshold(),
      getPhoneSettings(),
    ]);

  if (!cart || cart.lines.length === 0) {
    redirect(await withLocalePrefix("/cart", locale));
  }
  const checkout = await getCheckoutState(cart.id);
  if (!checkout) {
    redirect(await withLocalePrefix("/cart", locale));
  }

  const currency = channel?.currency ?? "USD";
  const loginHref = await withLocalePrefix(`/account/login?returnTo=${encodeURIComponent("/checkout")}`, locale);
  const registerHref = await withLocalePrefix(`/account/register?returnTo=${encodeURIComponent("/checkout")}`, locale);

  const progress: CheckoutProgress = {
    hasDestination: checkout.destination !== null,
    hasShippingMethod: checkout.shippingMethod !== null,
    detailsComplete: detailsComplete(
      {
        email: checkout.email,
        phone: checkout.phone,
        purchaseType: checkout.purchaseType,
        company: checkout.company,
        businessIds: Object.fromEntries(checkout.businessIds.map((b) => [b.key, b.value])),
        // The same reading the backend makes - the billing address's country, else where the order goes
        businessIdFields: businessIdFieldsFor(
          countries,
          companyCountry({
            billingCountry: (checkout.billingSameAsShipping ? checkout.shippingAddress : checkout.billingAddress)
              ?.countryCode ?? null,
            deliveryCountry: null,
            destinationCountry: checkout.destination?.countryCode ?? "",
          }),
          settings,
        ),
        isPickup: checkout.isPickup,
        hasShippingAddress: checkout.shippingAddress !== null,
        billingSameAsShipping: checkout.billingSameAsShipping,
        hasBillingAddress: checkout.billingAddress !== null,
        pickupContact: checkout.pickupContact,
      },
      settings,
    ),
  };
  const active = activeStep(progress, typeof requestedStep === "string" ? requestedStep : null);
  const status = (s: Parameters<typeof stepStatus>[0]) => stepStatus(s, active, progress);

  // Only the open step's options are fetched - each priced for this cart at its destination
  const [regionsByCountry, quotes, paymentMethods, savedCards] = await Promise.all([
    active === "details" ? getRegionsByCountry(countries.map((c) => c.code)) : {},
    active === "delivery" && checkout.destination ? getShippingQuotes(cart.id, checkout.destination) : [],
    active === "payment" && checkout.destination ? getPaymentMethods(checkout.destination.countryCode, cart.id) : [],
    active === "payment" && customer ? getMyPaymentMethods() : [],
  ]);
  const needStores = quotes.some((q) => q.carrierSlug === "pickup") || checkout.isPickup;
  const pickupLocations = needStores ? await getPickupLocations() : [];
  const pickupLocation = checkout.isPickup
    ? (pickupLocations.find((l) => `loc-${l.slug}` === checkout.shippingMethod?.code) ?? null)
    : null;

  // "Thu 3 Oct" for each quote the carrier estimates, formatted here so render and hydration agree on the day
  const arrivals: Record<string, string> = {};
  for (const q of quotes) {
    if (q.estimatedDays && q.estimatedDays > 0) {
      arrivals[selectionOf(q)] = format.dateTime(estimatedArrival(q.estimatedDays), {
        weekday: "short",
        day: "numeric",
        month: "short",
      });
    }
  }

  const defaultAddress = customer?.defaultShippingAddress ?? null;
  const edit = (step: string) => `/checkout?step=${step}`;
  const shownTotal =
    checkout.totals.giftCardCents + checkout.totals.storeCreditCents > 0
      ? checkout.totals.amountDueCents
      : checkout.totals.totalCents;

  return (
    <main className="mx-auto grid w-full max-w-8xl gap-10 px-4 pt-6 pb-12 sm:px-6 md:grid-cols-[1fr_22rem] md:px-10">
      <AnalyticsEvent
        enabled={consent.marketing}
        event="begin_checkout"
        payload={{
          ecommerce: {
            currency: typeof currency === "string" ? currency : currency.code,
            value: checkout.totals.totalCents / 100,
            items: cart.lines.map((line) => ({
              item_id: line.sku,
              item_name: displayName(line.name, line.sku),
              quantity: line.quantity,
              price: line.unitPrice / 100,
            })),
          },
        }}
      />
      <div className="min-w-0">
        <h1 className="text-3xl font-extrabold tracking-tight">{t("title")}</h1>

        {/* On a phone the summary would sit below every step - a collapsed bar at the top keeps the total in view */}
        <details className="glass mt-4 rounded-xl text-sm md:hidden">
          <summary className="flex cursor-pointer items-center justify-between gap-3 p-4 font-medium">
            <span>{t("showSummary")}</span>
            <span className="tabular-nums">{formatPrice(shownTotal, currency)}</span>
          </summary>
          <div className="border-t border-border p-4">
            <OrderSummary cart={cart} checkout={checkout} currency={currency} locale={locale} />
          </div>
        </details>

        {!customer && !guestAllowed ? (
          // The backend refuses guest placement outright when this is off (GUEST_CHECKOUT_DISABLED) -
          // saying so up front beats letting someone fill the whole form and get rejected at submit
          <div className="mt-6 border border-border p-6">
            <p className="text-sm font-medium">{t("accountRequired")}</p>
            <p className="mt-1 text-sm text-muted-foreground">{t("accountRequiredBody")}</p>
            <div className="mt-4 flex gap-3">
              <a
                href={loginHref}
                className="inline-flex h-11 items-center justify-center bg-foreground px-6 font-mono text-[13px] uppercase tracking-[0.14em] text-background"
              >
                {tc("signIn")}
              </a>
              <a
                href={registerHref}
                className="inline-flex h-11 items-center justify-center border border-foreground px-6 font-mono text-[13px] uppercase tracking-[0.14em]"
              >
                {t("createAccount")}
              </a>
            </div>
          </div>
        ) : customer ? (
          <p className="mt-2 text-sm text-muted-foreground">{t("signedInAs", { email: customer.email })}</p>
        ) : (
          <p className="mt-2 text-sm text-muted-foreground">
            {t.rich("asGuest", {
              link: (chunks) => (
                <a href={loginHref} className="underline underline-offset-2">
                  {chunks}
                </a>
              ),
            })}
          </p>
        )}

        {/* Gated: the steps are not rendered at all, so nobody fills in a checkout the API will refuse */}
        {(customer || guestAllowed) && (
          <div className="mt-8 grid gap-4">
            <StepCard
              number={1}
              title={t("steps.delivery")}
              status={status("delivery")}
              editLabel={tc("edit")}
              editHref={edit("delivery")}
              summary={<DeliverySummary checkout={checkout} currency={currency} free={t("delivery.free")} />}
            >
              <DeliveryStep
                destination={checkout.destination}
                suggested={{
                  countryCode: defaultAddress?.countryCode ?? countrySettings.defaultCountryCode,
                  postalCode: defaultAddress?.postalCode ?? "",
                }}
                countries={countries}
                quotes={quotes}
                arrivals={arrivals}
                selected={checkout.shippingMethod}
                pickupLocations={pickupLocations}
                thresholds={{
                  show: settings.thresholdProgress,
                  subtotalCents: cart.subtotal,
                  minOrderCents: settings.minOrderCents,
                  freeShippingCents: freeShipThreshold,
                }}
                notice={notice === "delivery-changed"}
                currency={currency}
              />
            </StepCard>

            <StepCard
              number={2}
              title={t("steps.details")}
              status={status("details")}
              editLabel={tc("edit")}
              editHref={edit("details")}
              summary={<DetailsSummary checkout={checkout} collectedBy={t("details.collectedBy")} />}
            >
              <DetailsStep
                checkout={{ ...checkout, phone: checkout.phone || customer?.phone || "" }}
                settings={settings}
                signedIn={!!customer}
                me={customer ? { firstName: customer.firstName, lastName: customer.lastName } : null}
                saved={customer?.addresses ?? []}
                defaultShippingId={defaultAddress?.id ?? null}
                defaultBillingId={customer?.defaultBillingAddress?.id ?? null}
                countries={countries}
                regionsByCountry={regionsByCountry}
                phoneSettings={phoneSettings}
                pickupLocation={pickupLocation}
                locale={locale}
              />
            </StepCard>

            <StepCard number={3} title={t("steps.payment")} status={status("payment")} editLabel={tc("edit")}>
              <PaymentStep
                methods={paymentMethods}
                checkout={checkout}
                note={cart.note}
                savedCards={savedCards}
                storeCreditBalance={customer?.storeCreditBalance ?? 0}
                signedIn={!!customer}
                consent={settings.termsConsent}
                locale={locale}
                currency={currency}
              />
            </StepCard>
          </div>
        )}
      </div>

      <aside className="glass hidden self-start rounded-xl p-6 text-sm md:sticky md:top-24 md:block">
        <h2 className="mb-4 text-xs font-medium uppercase tracking-wider text-muted-foreground">{t("summary")}</h2>
        <OrderSummary cart={cart} checkout={checkout} currency={currency} locale={locale} />
      </aside>
    </main>
  );
}

function DeliverySummary({
  checkout,
  currency,
  free,
}: {
  checkout: CheckoutState;
  currency: string | { code: string; symbol?: string; symbolPosition?: string };
  free: string;
}) {
  const m = checkout.shippingMethod;
  if (!m) return null;
  return (
    <>
      {m.name}
      <span> · {m.priceCents === 0 ? free : formatPrice(m.priceCents, currency)}</span>
      {!checkout.isPickup && checkout.destination ? (
        <span className="block text-xs">
          {checkout.destination.postalCode}, {checkout.destination.countryCode}
        </span>
      ) : null}
    </>
  );
}

function DetailsSummary({ checkout, collectedBy }: { checkout: CheckoutState; collectedBy: string }) {
  const p = checkout.pickupContact;
  return (
    <>
      <span className="block font-mono">{checkout.email}</span>
      {checkout.company ? <span className="block">{checkout.company}</span> : null}
      {checkout.isPickup && p ? (
        <span className="block">
          {collectedBy} {p.firstName} {p.lastName}
        </span>
      ) : checkout.shippingAddress ? (
        <span className="block">{oneLine(checkout.shippingAddress)}</span>
      ) : null}
    </>
  );
}
