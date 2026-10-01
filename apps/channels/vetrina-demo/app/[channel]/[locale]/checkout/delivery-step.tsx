"use client";

import { useTranslations } from "next-intl";
import { Fragment } from "react";

import { Field, FieldErrors, FormErrors } from "@/app/components/account/AccountUI";
import { FreeShippingBar } from "@/app/components/FreeShippingBar";
import { Link } from "@/app/components/LocaleLink";
import { saveDeliveryStep, saveDestinationStep } from "@/app/lib/checkout/actions";
import type { CheckoutDestination, PickupLocation, ShippingQuote } from "@/app/lib/checkout/read";
import { deliveryStepSchema, destinationSchema } from "@/app/lib/checkout/schemas";
import type { CountryOption } from "@/app/lib/countries/read";
import { formatPrice } from "@/app/lib/format";
import { useActionForm } from "@/app/lib/forms/use-action-form";

import { ChoiceCard, ContinueButton, CountrySelect, useReturnToNextStep } from "./fields";

type Currency = string | { code: string; symbol?: string; symbolPosition?: string };

const PICKUP_CARRIER = "pickup";
const isPickup = (q: ShippingQuote) => q.carrierSlug === PICKUP_CARRIER;
// Pickup codes are "loc-<slug>" - see the backend PickupCarrier
const pickupSlug = (q: ShippingQuote) => q.code.replace(/^loc-/, "");
export const selectionOf = (q: { carrierSlug: string; code: string }) => `${q.carrierSlug}::${q.code}`;

// The one delivery option that arrives soonest, when the carriers say - a tie marks none, since "fastest" of two equals
// tells the shopper nothing
export function fastestSelection(quotes: ShippingQuote[]): string | null {
  const timed = quotes.filter((q) => !isPickup(q) && (q.estimatedDays ?? 0) > 0);
  if (timed.length < 2) return null;
  const min = Math.min(...timed.map((q) => q.estimatedDays!));
  const soonest = timed.filter((q) => q.estimatedDays === min);
  return soonest.length === 1 ? selectionOf(soonest[0]) : null;
}

export type Thresholds = {
  show: boolean;
  subtotalCents: number;
  minOrderCents: number;
  freeShippingCents: number;
};

// How the order arrives, IKEA-style: a postcode first, then every delivery and pickup option for it on one list, each
// with its price and, where the carrier estimates one, the day it arrives
export function DeliveryStep({
  destination,
  suggested,
  countries,
  quotes,
  arrivals,
  selected,
  pickupLocations,
  thresholds,
  notice,
  currency,
}: {
  destination: CheckoutDestination | null;
  // Prefill when nothing is saved yet - a signed-in customer's default address, else the store's country
  suggested: { countryCode: string; postalCode: string };
  countries: CountryOption[];
  quotes: ShippingQuote[];
  // selection -> "Thu 3 Oct", formatted on the server so the date can't differ between render and hydration
  arrivals: Record<string, string>;
  selected: { carrierSlug: string; code: string } | null;
  pickupLocations: PickupLocation[];
  thresholds: Thresholds;
  notice: boolean;
  currency: Currency;
}) {
  const t = useTranslations("checkout.delivery");
  const belowMinimum = thresholds.minOrderCents > 0 && thresholds.subtotalCents < thresholds.minOrderCents;
  return (
    <div className="grid gap-5">
      {notice ? (
        <p role="status" className="rounded-lg border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900">
          {t("changedNotice")}
        </p>
      ) : null}
      <DestinationForm current={destination} suggested={suggested} countries={countries} />
      {thresholds.show && belowMinimum ? (
        <MinimumOrder thresholds={thresholds} currency={currency} />
      ) : null}
      {destination && !belowMinimum ? (
        <MethodForm
          quotes={quotes}
          arrivals={arrivals}
          selected={selected}
          pickupLocations={pickupLocations}
          currency={currency}
        />
      ) : null}
      {thresholds.show && !belowMinimum && destination && quotes.some((q) => !isPickup(q)) ? (
        <FreeShippingBar
          subtotal={thresholds.subtotalCents}
          thresholdCents={thresholds.freeShippingCents}
          currency={currency}
        />
      ) : null}
    </div>
  );
}

function DestinationForm({
  current,
  suggested,
  countries,
}: {
  current: CheckoutDestination | null;
  suggested: { countryCode: string; postalCode: string };
  countries: CountryOption[];
}) {
  const t = useTranslations("checkout.delivery");
  const { form, fields, formAction, onSubmit, pending } = useActionForm(saveDestinationStep, destinationSchema, {
    defaultValue: {
      countryCode: current?.countryCode ?? suggested.countryCode,
      postalCode: current?.postalCode ?? suggested.postalCode,
    },
  });
  // One country needs no picker - it posts as a hidden field
  const single = countries.length === 1 ? countries[0].code : null;
  return (
    <form id={form.id} onSubmit={onSubmit} action={formAction} noValidate className="grid gap-3">
      <p className="text-sm text-muted-foreground">{t("postcodeIntro")}</p>
      <FormErrors errors={form.errors} />
      <div className={`grid items-start gap-3 ${single ? "sm:grid-cols-[1fr_auto]" : "sm:grid-cols-[1fr_1fr_auto]"}`}>
        {single ? (
          <input type="hidden" name={fields.countryCode.name} value={single} />
        ) : (
          <CountrySelect field={fields.countryCode} countries={countries} autoComplete="shipping country" />
        )}
        <Field label={t("postcode")} field={fields.postalCode} autoComplete="shipping postal-code" required />
        {/* An empty label row above the button lines it up with the inputs, and keeps it there when an error opens
            below the postcode. Same border, padding and text size as the inputs, so the same height */}
        <div className="flex flex-col gap-1.5 text-sm">
          <span aria-hidden className="invisible hidden font-medium sm:block">
            {t("postcode")}
          </span>
          <button
            type="submit"
            disabled={pending}
            className="inline-flex items-center justify-center rounded-lg border border-border px-5 py-2 text-sm font-semibold transition-colors hover:bg-muted disabled:opacity-50"
          >
            {pending ? t("finding") : current ? t("update") : t("showOptions")}
          </button>
        </div>
      </div>
    </form>
  );
}

function MethodForm({
  quotes,
  arrivals,
  selected,
  pickupLocations,
  currency,
}: {
  quotes: ShippingQuote[];
  arrivals: Record<string, string>;
  selected: { carrierSlug: string; code: string } | null;
  pickupLocations: PickupLocation[];
  currency: Currency;
}) {
  const t = useTranslations("checkout.delivery");
  // Delivery before pickup, cheapest first in each - and the cheapest overall pre-picked when nothing is chosen yet
  const ordered = quotes
    .slice()
    .sort((a, b) => Number(isPickup(a)) - Number(isPickup(b)) || a.priceCents - b.priceCents);
  const cheapest = quotes.slice().sort((a, b) => a.priceCents - b.priceCents)[0];
  const initial = selected ? selectionOf(selected) : cheapest ? selectionOf(cheapest) : "";
  const fastest = fastestSelection(quotes);
  const { form, fields, formAction, onSubmit, pending, lastResult } = useActionForm(
    saveDeliveryStep,
    deliveryStepSchema,
    { defaultValue: { selection: initial } },
  );
  useReturnToNextStep(lastResult);

  if (quotes.length === 0) {
    return <p className="text-sm text-muted-foreground">{t("none")}</p>;
  }
  const pickupBySlug = new Map(pickupLocations.map((l) => [l.slug, l]));
  const bothKinds = ordered.some(isPickup) && ordered.some((q) => !isPickup(q));
  return (
    <form id={form.id} onSubmit={onSubmit} action={formAction} noValidate className="grid gap-3">
      <FormErrors errors={form.errors} />
      <fieldset>
        <legend className="sr-only">{t("legend")}</legend>
        <ul className="space-y-2" aria-live="polite">
          {ordered.map((q, i) => {
            // Group headings only when both kinds are offered - one heading over one list is noise
            const heading =
              bothKinds && (i === 0 || isPickup(ordered[i - 1]) !== isPickup(q))
                ? isPickup(q)
                  ? t("pickupGroup")
                  : t("deliveryGroup")
                : null;
            const loc = isPickup(q) ? pickupBySlug.get(pickupSlug(q)) : undefined;
            const sel = selectionOf(q);
            return (
              <Fragment key={sel}>
                {heading ? (
                  <li className="pt-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground first:pt-0">
                    {heading}
                  </li>
                ) : null}
                <li>
                  <ChoiceCard
                    name={fields.selection.name}
                    value={sel}
                    defaultChecked={initial === sel}
                    aside={q.priceCents === 0 ? t("free") : formatPrice(q.priceCents, currency)}
                  >
                    {loc ? (
                      <>
                        <span className="font-medium">{loc.name}</span>
                        <span className="mt-0.5 block text-xs text-muted-foreground">
                          {[loc.line1, loc.city, loc.postalCode].filter(Boolean).join(", ")}
                        </span>
                        {loc.instructions ? (
                          <span className="mt-1 block text-xs text-muted-foreground">{loc.instructions}</span>
                        ) : null}
                      </>
                    ) : (
                      <>
                        <span className="font-medium">{q.name}</span>
                        <span className="text-muted-foreground"> · {q.carrierName}</span>
                        {fastest === sel ? (
                          <span className="ml-2 rounded-full bg-foreground px-2 py-0.5 text-[11px] font-semibold text-background">
                            {t("fastest")}
                          </span>
                        ) : null}
                        {arrivals[sel] ? (
                          <span className="mt-0.5 block text-xs text-muted-foreground">
                            {t("arrives", { date: arrivals[sel] })}
                          </span>
                        ) : null}
                      </>
                    )}
                  </ChoiceCard>
                </li>
              </Fragment>
            );
          })}
        </ul>
      </fieldset>
      {fields.selection.errors ? (
        <FieldErrors id={fields.selection.errorId} errors={fields.selection.errors} />
      ) : null}
      <ContinueButton pending={pending} label={t("continueToDetails")} />
    </form>
  );
}

// Below the store's minimum, a goal rather than a refusal at the last step - how much more, and the way back to the cart
function MinimumOrder({ thresholds, currency }: { thresholds: Thresholds; currency: Currency }) {
  const t = useTranslations("checkout.delivery");
  const remaining = thresholds.minOrderCents - thresholds.subtotalCents;
  const pct = Math.max(0, Math.min(100, Math.round((thresholds.subtotalCents / thresholds.minOrderCents) * 100)));
  return (
    <div className="rounded-lg border border-border p-4 text-sm">
      <p className="font-semibold">{t("minimumTitle", { amount: formatPrice(thresholds.minOrderCents, currency) })}</p>
      <div
        className="mt-2 h-1.5 w-full overflow-hidden rounded-full bg-border"
        role="progressbar"
        aria-valuenow={pct}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-label={t("minimumProgress")}
      >
        <div className="h-full rounded-full bg-amber-500" style={{ width: `${pct}%` }} />
      </div>
      <p className="mt-2 text-muted-foreground">
        {t.rich("minimumRemaining", {
          amount: () => <span className="font-medium text-foreground">{formatPrice(remaining, currency)}</span>,
        })}
      </p>
      <Link href="/cart" className="mt-2 inline-block text-xs underline underline-offset-2">
        {t("backToCart")}
      </Link>
    </div>
  );
}
