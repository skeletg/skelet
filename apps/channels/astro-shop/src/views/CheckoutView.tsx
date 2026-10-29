import type { CheckoutQuery, CountriesQuery, PaymentOptionsQuery } from "../graphql/checkout.gql";
import type { Channel } from "../lib/channel";
import type { Currency } from "../lib/format";
import { Choice } from "../components/Choice";
import { Field } from "../components/Field";
import { Price } from "../components/Price";
import { Button, Notice } from "../components/ui";

type Cart = NonNullable<CheckoutQuery["cart"]>;
type Group = Cart["checkout"]["deliveryGroups"][number];
// A door-delivery choice with the "option|carrier|method" value its radio posts
export type DeliveryGroup = Omit<Group, "choices"> & { choices: (Group["choices"][number] & { value: string })[] };

// What each input shows: what was just posted, else what the cart has saved
export type CheckoutValues = {
  countryCode: string;
  postalCode: string;
  email: string;
  firstName: string;
  lastName: string;
  street1: string;
  street2: string;
  city: string;
  state: string;
  phone: string;
  acceptTerms: boolean;
};

type Props = {
  error?: string;
  cart: Cart;
  channel: Channel;
  countries: CountriesQuery["countries"];
  // Null until the destination step is done (or while it's being changed)
  dest: { countryCode: string; postalCode: string } | null;
  groups: DeliveryGroup[];
  // The picked delivery value per group, and the picked payment provider
  picks: Map<string, string | undefined>;
  payments: PaymentOptionsQuery["paymentMethods"];
  pickedPayment?: string;
  values: CheckoutValues;
};

const sectionTitle = "mb-4 flex items-center gap-3 text-lg font-semibold";
const stepDot = "grid size-7 place-items-center rounded-full bg-primary text-xs font-bold text-primary-foreground";

function StepTitle({ n, children }: { n: number; children: string }) {
  return (
    <h2 className={sectionTitle}>
      <span className={stepDot}>{n}</span>
      {children}
    </h2>
  );
}

export function CheckoutView({ error, cart, channel, countries, dest, groups, picks, payments, pickedPayment, values }: Props) {
  return (
    <>
      <h1 className="mb-8 text-3xl font-semibold tracking-tight sm:text-4xl">Checkout</h1>
      {error && <Notice>{error}</Notice>}

      <div className="grid items-start gap-10 lg:grid-cols-[1fr_380px]">
        <div>
          {!dest ? (
            <DestinationForm countries={countries} values={values} />
          ) : (
            <form method="post" id="checkout-details" className="grid gap-10">
              <input type="hidden" name="countryCode" value={dest.countryCode} />
              <input type="hidden" name="postalCode" value={dest.postalCode} />
              <DeliverySection dest={dest} countries={countries} groups={groups} picks={picks} channel={channel} />
              <ContactSection values={values} />
              <PaymentSection payments={payments} pickedPayment={pickedPayment} />
              <div className="grid gap-4">
                <label className="flex items-center gap-3 text-sm">
                  <input type="checkbox" name="acceptTerms" required defaultChecked={values.acceptTerms} className="size-5 min-h-0 w-auto accent-primary" />
                  I accept the terms and conditions
                </label>
                <Button type="submit" name="intent" value="place" className="w-full text-lg" disabled={payments.length === 0 || groups.some((g) => g.choices.length === 0)}>
                  Place order
                </Button>
              </div>
            </form>
          )}
        </div>
        <OrderSummary cart={cart} delivering={!!dest} currency={channel.currency} />
      </div>
    </>
  );
}

function DestinationForm({ countries, values }: Pick<Props, "countries" | "values">) {
  return (
    <form method="post" className="grid gap-6 rounded-3xl border border-border bg-card p-6 sm:p-8">
      <StepTitle n={1}>Where should we send it?</StepTitle>
      <input type="hidden" name="intent" value="destination" />
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Country">
          <select name="countryCode" required autoComplete="country" defaultValue={values.countryCode}>
            {countries.map((x) => (
              <option key={x.code} value={x.code}>
                {x.name}
              </option>
            ))}
          </select>
        </Field>
        <Field label="Postal code">
          <input name="postalCode" required autoComplete="postal-code" defaultValue={values.postalCode} />
        </Field>
      </div>
      <Button type="submit" className="justify-self-start">
        Continue
      </Button>
    </form>
  );
}

function DeliverySection({ dest, countries, groups, picks, channel }: Pick<Props, "countries" | "groups" | "picks" | "channel"> & { dest: NonNullable<Props["dest"]> }) {
  const country = countries.find((x) => x.code === dest.countryCode);
  return (
    <section>
      <StepTitle n={1}>Delivery</StepTitle>
      <p className="-mt-2 mb-4 text-sm text-muted-foreground">
        To {country?.name ?? dest.countryCode} {dest.postalCode} ·{" "}
        <a href="/checkout?step=destination" className="text-foreground underline underline-offset-4">
          Change
        </a>
      </p>
      {groups.map((g) => (
        <fieldset key={g.key} className="grid gap-2">
          <legend className="sr-only">Delivery option</legend>
          <input type="hidden" name="group" value={g.key} />
          {g.choices.length === 0 && <Notice>These items can't be delivered there.</Notice>}
          {g.choices.map((ch) => (
            <Choice key={ch.value} name={`delivery:${g.key}`} value={ch.value} checked={ch.value === picks.get(g.key)} autosubmit="update-shipping">
              <span className="flex-1">
                {ch.methodName || ch.optionName}
                {ch.estimatedArrival && (
                  <span className="block text-sm text-muted-foreground">
                    Arrives {new Date(ch.estimatedArrival).toLocaleDateString(channel.defaultLocale, { weekday: "short", day: "numeric", month: "short" })}
                  </span>
                )}
              </span>
              <span className="font-medium">{ch.priceCents === 0 ? "Free" : <Price amount={ch.priceCents} currency={channel.currency} />}</span>
            </Choice>
          ))}
        </fieldset>
      ))}
    </section>
  );
}

function ContactSection({ values: v }: Pick<Props, "values">) {
  return (
    <section>
      <StepTitle n={2}>Contact and address</StepTitle>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Email" wide>
          <input type="email" name="email" required autoComplete="email" defaultValue={v.email} />
        </Field>
        <Field label="First name">
          <input name="firstName" required autoComplete="given-name" defaultValue={v.firstName} />
        </Field>
        <Field label="Last name">
          <input name="lastName" required autoComplete="family-name" defaultValue={v.lastName} />
        </Field>
        <Field label="Address" wide>
          <input name="street1" required autoComplete="address-line1" defaultValue={v.street1} />
        </Field>
        <Field label="Apartment, suite" optional wide>
          <input name="street2" autoComplete="address-line2" defaultValue={v.street2} />
        </Field>
        <Field label="City">
          <input name="city" required autoComplete="address-level2" defaultValue={v.city} />
        </Field>
        <Field label="State / region">
          <input name="state" autoComplete="address-level1" defaultValue={v.state} />
        </Field>
        <Field label="Phone" optional wide>
          <input type="tel" name="phone" autoComplete="tel" defaultValue={v.phone} />
        </Field>
      </div>
    </section>
  );
}

function PaymentSection({ payments, pickedPayment }: Pick<Props, "payments" | "pickedPayment">) {
  return (
    <section>
      <StepTitle n={3}>Payment</StepTitle>
      {payments.length === 0 && <Notice>No payment method is available for this country.</Notice>}
      <fieldset className="grid gap-2">
        <legend className="sr-only">Payment method</legend>
        {payments.map((p) => (
          <Choice key={p.providerSlug} name="payment" value={p.providerSlug} checked={p.providerSlug === pickedPayment}>
            <span className="flex-1">
              {p.displayName}
              {p.instructions && <span className="block text-sm text-muted-foreground">{p.instructions}</span>}
            </span>
          </Choice>
        ))}
      </fieldset>
    </section>
  );
}

// Swapped in after a delivery pick (enhance.ts), so shipping and the total follow the choice
function OrderSummary({ cart, delivering, currency }: { cart: Cart; delivering: boolean; currency: Currency }) {
  const t = cart.checkout.totals;
  return (
    <aside className="rounded-3xl border border-border bg-card p-6 lg:sticky lg:top-24" aria-label="Order summary" data-summary="">
      <h2 className="mb-4 text-lg font-semibold">Order summary</h2>
      <ul className="grid gap-3 border-b border-border pb-4">
        {cart.lines.map((l) => (
          <li key={l.id} className="flex justify-between gap-3 text-sm">
            <span>
              {l.name} <span className="text-muted-foreground">× {l.quantity}</span>
            </span>
            <Price amount={l.totalPrice} currency={currency} />
          </li>
        ))}
      </ul>
      <dl className="mt-4 grid gap-2 text-sm">
        <div className="flex justify-between">
          <dt className="text-muted-foreground">Subtotal</dt>
          <dd>
            <Price amount={t.subtotalCents} currency={currency} />
          </dd>
        </div>
        {t.discountCents > 0 && (
          <div className="flex justify-between text-success">
            <dt>Discount</dt>
            <dd>
              −<Price amount={t.discountCents} currency={currency} />
            </dd>
          </div>
        )}
        {delivering && (
          <div className="flex justify-between">
            <dt className="text-muted-foreground">Shipping</dt>
            <dd>{t.shippingCents === 0 ? "Free" : <Price amount={t.shippingCents} currency={currency} />}</dd>
          </div>
        )}
        {t.taxCents > 0 && (
          <div className="flex justify-between">
            <dt className="text-muted-foreground">Tax</dt>
            <dd>
              <Price amount={t.taxCents} currency={currency} />
            </dd>
          </div>
        )}
        <div className="mt-2 flex justify-between border-t border-border pt-3 text-base font-semibold">
          <dt>Total</dt>
          <dd data-total="">
            <Price amount={t.totalCents} currency={currency} />
          </dd>
        </div>
      </dl>
      {delivering && (
        // Without scripts a delivery pick reprices through this; with them, each pick submits through it on its own
        <button
          type="submit"
          id="update-shipping"
          form="checkout-details"
          name="intent"
          value="delivery"
          formNoValidate
          data-enhance="[data-summary]"
          className="mt-4 text-sm text-foreground underline underline-offset-4 js:hidden"
        >
          Update shipping
        </button>
      )}
    </aside>
  );
}
