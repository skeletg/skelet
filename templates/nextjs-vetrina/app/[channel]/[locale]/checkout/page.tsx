import type { Metadata } from "next";
import { getFormatter, getTranslations } from "next-intl/server";
import { redirect } from "next/navigation";

import { withLocalePrefix } from "@skelet/storefront-next/locale/path";

import { CheckoutCartProvider } from "@/app/components/offer/CheckoutCartContext";
import { checkoutHref } from "@skelet/storefront-next/checkout/path";
import { AnalyticsEvent } from "@/app/components/AnalyticsEvent";
import { formatPrice, getFreeShippingThreshold } from "@skelet/storefront-next/cart/read";
import { getCheckoutCart } from "@skelet/storefront-next/cart/offer";
import { offerLocks, type CartOfferLock } from "@skelet/storefront-next/cart/types";
import { OwnCartLink } from "@/app/components/offer/LeaveOfferCheckout";
import { detailsAsks } from "@skelet/storefront-next/checkout/asks";
import { choiceKey, choiceLabel } from "@skelet/storefront-next/checkout/choice";
import {
  getCheckoutState,
  getDeliveryPoints,
  checkoutPaymentMethods,
  type CheckoutState,
  type DeliveryPointsResult,
} from "@skelet/storefront-next/checkout/read";
import { businessIdFieldsFor, companyCountry } from "@skelet/storefront-next/checkout/business-ids";
import { activeStep, arrivalLabels, businessComplete, progressOf, stepsFor, stepStatus } from "@skelet/storefront-next/checkout/step";
import { getResolvedConsent } from "@skelet/storefront-next/consent/read";
import { getCountries, getCountrySettings, getRegionsByCountry, getPhoneSettings } from "@skelet/storefront-next/countries/read";
import { getCheckoutSettings, getGuestCheckoutAllowed } from "@skelet/storefront-next/config/read";
import { displayName } from "@skelet/storefront-next/display";
import { getCurrentCustomer } from "@skelet/storefront-next/customer/read";
import { getMyPaymentMethods } from "@skelet/storefront-next/payment-methods/actions";
import { getChannel } from "@skelet/storefront-next/channel/read";
import { toMajor } from "@skelet/storefront-next/format";

import { DeliveryStep, type LineInfo } from "./delivery-step";
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
  const { step: requestedStep, notice, pointGroup, pointSearch, offer: requestedOffer } = await searchParams;
  const offerId = typeof requestedOffer === "string" ? requestedOffer : undefined;
  const offerCheckout = !!offerId;
  const t = await getTranslations({ locale, namespace: "checkout" });
  const tc = await getTranslations({ locale, namespace: "common" });
  const to = await getTranslations({ locale, namespace: "offer" });
  const format = await getFormatter({ locale });

  const [customer, cart, channel, consent, countries, countrySettings, guestAllowed, settings, freeShipThreshold, phoneSettings] =
    await Promise.all([
      getCurrentCustomer(),
      getCheckoutCart(offerId),
      getChannel(),
      getResolvedConsent(),
      getCountries(),
      getCountrySettings(),
      getGuestCheckoutAllowed(),
      getCheckoutSettings(),
      getFreeShippingThreshold(),
      getPhoneSettings(),
    ]);

  const cartHref = offerId ? `/cart/offer?cartId=${encodeURIComponent(offerId)}` : "/cart";
  if (!cart || cart.lines.length === 0 || (offerCheckout && !cart.offer)) {
    redirect(await withLocalePrefix(cartHref, locale));
  }
  // An offer is the store's own terms - it checks out whatever the guest-checkout setting says, and what it locks
  // isn't offered for editing here (the backend refuses a change to it either way)
  const offer = offerCheckout ? (cart.offer ?? null) : null;
  const deliveryLocked = offerLocks(cart, "DELIVERY");
  const lockedParts = offer
    ? (["LINES", "SHIPPING", "BILLING", "DELIVERY", "COUPONS"] as CartOfferLock[])
        .filter((l) => offer.locks.includes(l))
        .map((l) => to(`lock.${l}`))
    : [];
  const checkout = await getCheckoutState(cart.id);
  if (!checkout) {
    redirect(await withLocalePrefix(cartHref, locale));
  }

  const currency = channel?.currency ?? "USD";
  const loginHref = await withLocalePrefix(`/account/login?returnTo=${encodeURIComponent(checkoutHref(offerId))}`, locale);
  const registerHref = await withLocalePrefix(`/account/register?returnTo=${encodeURIComponent(checkoutHref(offerId))}`, locale);

  const businessOk = businessComplete(
    {
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
    },
    settings,
  );
  const progress = progressOf(checkout, businessOk);
  const reopened = typeof requestedStep === "string" && !(deliveryLocked && requestedStep === "delivery") ? requestedStep : null;
  const active = activeStep(progress, reopened);
  const status = (s: Parameters<typeof stepStatus>[0]) => stepStatus(s, active, progress);
  const steps = stepsFor(progress);
  const asks = detailsAsks(checkout.requirements);

  // What a group's chosen delivery asks next - its points - only while the delivery step is open
  const asked = (key: string, type: string) => checkout.requirements.some((r) => r.scope === key && r.type === type);
  const pointGroups = active === "delivery" ? checkout.deliveryGroups.filter((g) => g.selection && asked(g.key, "point")) : [];
  const [regionsByCountry, pointLists, paymentMethods, savedCards] = await Promise.all([
    active === "details" ? getRegionsByCountry(countries.map((c) => c.code)) : {},
    Promise.all(pointGroups.map((g) => getDeliveryPoints(cart.id, g.key, pointGroup === g.key && typeof pointSearch === "string" ? pointSearch : undefined))),
    active === "payment" ? checkoutPaymentMethods(checkout, cart.id, offer?.paymentMethods) : [],
    active === "payment" && customer ? getMyPaymentMethods() : [],
  ]);
  const points: Record<string, DeliveryPointsResult> = Object.fromEntries(pointGroups.map((g, i) => [g.key, pointLists[i]]));
  // Where each pickup is collected from, for the details step's "who collects" block
  const collectFrom = checkout.deliveryGroups.flatMap((g) =>
    g.selection?.point && asked(g.key, "collector") ? [g.selection.point] : [],
  );

  // Formatted here so render and hydration agree on the day
  const arrivals = arrivalLabels(
    checkout.deliveryGroups.flatMap((g) => g.choices),
    (date, options) => format.dateTime(date, options),
  );
  const lines: Record<string, LineInfo> = Object.fromEntries(
    cart.lines.map((l) => [l.id, { id: l.id, name: displayName(l.name, l.sku), quantity: l.quantity, imageUrl: l.imageUrl }]),
  );
  const n = (step: (typeof steps)[number]) => steps.indexOf(step) + 1;

  const defaultAddress = customer?.defaultShippingAddress ?? null;
  const edit = (step: string) => checkoutHref(offerId, { step });
  const shownTotal =
    checkout.totals.giftCardCents + checkout.totals.storeCreditCents > 0
      ? checkout.totals.amountDueCents
      : checkout.totals.totalCents;

  return (
    <CheckoutCartProvider cartId={cart.id} offerId={offer ? cart.id : undefined}>
    <main className="mx-auto grid w-full max-w-8xl gap-10 px-4 pt-6 pb-12 sm:px-6 md:grid-cols-[1fr_22rem] md:px-10">
      <AnalyticsEvent
        enabled={consent.marketing}
        event="begin_checkout"
        payload={{
          ecommerce: {
            currency: typeof currency === "string" ? currency : currency.code,
            value: toMajor(checkout.totals.totalCents, currency),
            items: cart.lines.map((line) => ({
              item_id: line.sku,
              item_name: displayName(line.name, line.sku),
              quantity: line.quantity,
              price: toMajor(line.unitPrice, currency),
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

        {offer ? (
          <div className="mt-4 rounded-xl border border-border p-4 text-sm">
            <p>{to("checkoutBanner")}</p>
            {lockedParts.length > 0 ? <p className="mt-1 text-muted-foreground">{to("locked", { parts: lockedParts.join(", ") })}</p> : null}
            <OwnCartLink href="/cart" className="mt-2 inline-block underline underline-offset-2">
              {to("backToCart")}
            </OwnCartLink>
          </div>
        ) : null}

        {!customer && !guestAllowed && !offer ? (
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
        {(customer || guestAllowed || offer) && (
          <div className="mt-8 grid gap-4">
            {progress.needsDelivery ? (
              <StepCard
                number={n("delivery")}
                title={t("steps.delivery")}
                status={status("delivery")}
                editLabel={tc("edit")}
                editHref={deliveryLocked ? undefined : edit("delivery")}
                summary={
                  <>
                    <DeliverySummary checkout={checkout} currency={currency} free={t("delivery.free")} />
                    {deliveryLocked ? <span className="block text-xs">{to("lockedStep")}</span> : null}
                  </>
                }
              >
                <DeliveryStep
                  checkout={checkout}
                  suggested={{
                    countryCode: defaultAddress?.countryCode ?? countrySettings.defaultCountryCode,
                    postalCode: defaultAddress?.postalCode ?? "",
                  }}
                  countries={countries}
                  lines={lines}
                  arrivals={arrivals}
                  points={points}
                  pointSearch={{ group: typeof pointGroup === "string" ? pointGroup : "", query: typeof pointSearch === "string" ? pointSearch : "" }}
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
            ) : null}

            <StepCard
              number={n("details")}
              title={t("steps.details")}
              status={status("details")}
              editLabel={tc("edit")}
              editHref={edit("details")}
              summary={<DetailsSummary checkout={checkout} asks={asks} collectedBy={t("details.collectedBy")} />}
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
                asks={asks}
                collectFrom={collectFrom}
                locale={locale}
              />
            </StepCard>

            <StepCard number={n("payment")} title={t("steps.payment")} status={status("payment")} editLabel={tc("edit")}>
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
    </CheckoutCartProvider>
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
  // Named the way its card was - the option, then the carrier or method when the option offered several
  const chosen = checkout.deliveryGroups.flatMap((g) => {
    if (!g.selection) return [];
    const sel = g.selection;
    const choice = g.choices.find((c) => choiceKey(c) === choiceKey(sel));
    return [{ sel, label: choice ? choiceLabel(choice, g.choices).join(" · ") : sel.optionName }];
  });
  if (chosen.length === 0) return null;
  return (
    <>
      {chosen.map(({ sel, label }) => (
        <span key={sel.optionId + sel.methodCode + (sel.point?.id ?? "")} className="block">
          {label}
          {sel.point ? ` - ${sel.point.name}` : ""}
          <span> · {sel.priceCents === 0 ? free : formatPrice(sel.priceCents, currency)}</span>
        </span>
      ))}
      {checkout.destination ? (
        <span className="block text-xs">
          {checkout.destination.postalCode}, {checkout.destination.countryCode}
        </span>
      ) : null}
    </>
  );
}

function DetailsSummary({
  checkout,
  asks,
  collectedBy,
}: {
  checkout: CheckoutState;
  asks: ReturnType<typeof detailsAsks>;
  collectedBy: string;
}) {
  const p = checkout.pickupContact;
  const billing = checkout.billingAddress;
  return (
    <>
      <span className="block font-mono">{checkout.email}</span>
      {checkout.company ? <span className="block">{checkout.company}</span> : null}
      {asks.collector && p ? (
        <span className="block">
          {collectedBy} {p.firstName} {p.lastName}
        </span>
      ) : null}
      {asks.shipping && checkout.shippingAddress ? (
        <span className="block">{oneLine(checkout.shippingAddress)}</span>
      ) : !asks.shipping && billing ? (
        <span className="block">{oneLine(billing)}</span>
      ) : null}
    </>
  );
}
