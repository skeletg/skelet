"use client";

import { useTranslations } from "next-intl";
import { useMemo } from "react";

import { Field, FieldErrors, FormErrors } from "@/app/components/account/AccountUI";
import { saveDetailsStep } from "@/app/lib/checkout/actions";
import type { DetailsAsks } from "@/app/lib/checkout/asks";
import type { CheckoutAddress, CheckoutState, DeliveryPoint } from "@/app/lib/checkout/read";
import { businessIdFieldsFor, GENERIC_TAX_ID } from "@/app/lib/checkout/business-ids";
import { billingIsShown, detailsCompanyCountry, detailsStepSchema, type DetailsMode } from "@/app/lib/checkout/schemas";
import type { CheckoutSettings } from "@/app/lib/checkout/settings";
import type { BusinessIdField, CountryOption, PhoneSettings, RegionOption } from "@/app/lib/countries/read";
import { phoneFieldFor } from "@/app/lib/forms/phone";
import type { Address } from "@/app/lib/customer/read";
import { useActionForm } from "@/app/lib/forms/use-action-form";

import { AddressFields, ChoiceCard, ContinueButton, useReturnToNextStep, type ValueField } from "./fields";
import { AddressLines } from "./summary";

type Props = {
  checkout: CheckoutState;
  settings: CheckoutSettings;
  signedIn: boolean;
  // The signed-in customer's name, the pickup contact's starting point when they collect themselves
  me: { firstName: string; lastName: string } | null;
  saved: Address[]; // the customer's address book, empty for guests
  defaultShippingId: string | null;
  defaultBillingId: string | null;
  countries: CountryOption[];
  regionsByCountry: Record<string, RegionOption[]>;
  // The store's phone policy - the contact phone is read in the destination's country unless the store fixes one
  phoneSettings: PhoneSettings;
  // What the chosen deliveries ask of the shopper, from the cart's requirements
  asks: DetailsAsks;
  // Where a pickup is collected from - shown above who collects it
  collectFrom: DeliveryPoint[];
  locale: string;
};

// A typed-in address starts from what the cart holds when that was typed too, else from the destination the first step
// saved - the postcode and country are already known
function addressDefaults(current: CheckoutAddress | null, fallback: { countryCode: string; postalCode: string }) {
  const typed = current && !current.sourceAddressId ? current : null;
  return {
    firstName: typed?.firstName ?? "",
    lastName: typed?.lastName ?? "",
    company: typed?.company ?? "",
    street1: typed?.street1 ?? "",
    street2: typed?.street2 ?? "",
    city: typed?.city ?? "",
    state: typed?.state ?? "",
    postalCode: typed?.postalCode ?? fallback.postalCode,
    countryCode: typed?.countryCode ?? fallback.countryCode,
  };
}

// Step 2 - who the order is for, in one form. What it asks follows the chosen deliveries: their address when one ships
// to the shopper (billing inline beneath it), who collects when one is a pickup, a billing address as strictly as any
// delivery - or a download - needs one. A PO box asks neither address nor collector, only billing
export function DetailsStep(props: Props) {
  const { checkout, settings, signedIn, me, saved, defaultShippingId, defaultBillingId, countries, regionsByCountry } =
    props;
  const t = useTranslations("checkout.details");
  const { asks } = props;
  const dest = {
    countryCode: checkout.destination?.countryCode ?? "",
    postalCode: checkout.destination?.postalCode ?? "",
  };
  const mode = useMemo<DetailsMode>(
    () => ({
      asks,
      signedIn,
      countries,
      savedCountries: Object.fromEntries(saved.map((a) => [a.id, a.countryCode])),
      destinationCountry: dest.countryCode,
    }),
    [asks, signedIn, countries, saved, dest.countryCode],
  );
  const schema = useMemo(() => detailsStepSchema(settings, mode), [settings, mode]);

  const shippingChoice = checkout.shippingAddress
    ? (checkout.shippingAddress.sourceAddressId ?? "new")
    : (defaultShippingId ?? saved[0]?.id ?? "new");
  const billingChoice = checkout.billingAddress
    ? (checkout.billingAddress.sourceAddressId ?? "new")
    : (defaultBillingId ?? saved[0]?.id ?? "new");
  // Saving this step always sets the delivery address, so without one billingSameAsShipping is still the cart's
  // default - a customer whose default billing address isn't the one being delivered to starts with it open
  const billingSame = checkout.shippingAddress
    ? checkout.billingSameAsShipping
    : !(defaultBillingId && defaultBillingId !== shippingChoice);
  const contact = checkout.pickupContact;

  const { form, fields, formAction, onSubmit, pending, lastResult } = useActionForm(saveDetailsStep, schema, {
    defaultValue: {
      email: checkout.email,
      phone: checkout.phone,
      phoneCountry: dest.countryCode,
      purchaseType: checkout.purchaseType === "BUSINESS" ? "business" : "personal",
      company: checkout.company,
      businessIds: Object.fromEntries(checkout.businessIds.map((b) => [b.key, b.value])),
      poNumber: checkout.poNumber,
      shippingChoice,
      shipping: addressDefaults(checkout.shippingAddress, dest),
      billingSame: asks.shipping ? billingSame : false,
      addBilling: !asks.shipping && checkout.billingAddress !== null,
      billingChoice,
      billing: addressDefaults(checkout.billingSameAsShipping ? null : checkout.billingAddress, dest),
      pickupWho: contact?.someoneElse ? "someoneElse" : "me",
      pickupFirstName: contact?.firstName ?? me?.firstName ?? "",
      pickupLastName: contact?.lastName ?? me?.lastName ?? "",
    },
  });
  useReturnToNextStep(lastResult);

  const business = settings.business && fields.purchaseType.value === "business";
  const someoneElse = asks.collector && settings.pickupSomeoneElse && fields.pickupWho.value === "someoneElse";
  const billingOwed = !asks.shipping && (asks.billing === "required" || someoneElse);
  const showBilling = billingIsShown(asks, someoneElse, {
    billingSame: fields.billingSame.value === "on",
    addBilling: fields.addBilling.value === "on",
  });
  const shipping = fields.shipping.getFieldset() as unknown as Record<string, ValueField>;
  const billing = fields.billing.getFieldset() as unknown as Record<string, ValueField>;
  // The identifiers follow the company's country as the form has it now - a billing address picked or typed below
  // swaps them for that country's
  const text = (f: ValueField | undefined) => (typeof f?.value === "string" ? f.value : "");
  const idCountry = detailsCompanyCountry(
    {
      shippingChoice: text(fields.shippingChoice) || shippingChoice,
      shipping: { countryCode: text(shipping.countryCode) },
      billingChoice: text(fields.billingChoice) || billingChoice,
      billing: { countryCode: text(billing.countryCode) },
      billingSame: !showBilling,
    },
    mode,
    showBilling,
  );
  const idFields = businessIdFieldsFor(countries, idCountry, settings);

  return (
    <form id={form.id} onSubmit={onSubmit} action={formAction} noValidate className="grid gap-5">
      <input type="hidden" name="locale" value={props.locale} />
      <FormErrors errors={form.errors} />

      {settings.business ? (
        <BusinessFields fields={fields} settings={settings} business={business} idFields={idFields} />
      ) : null}

      <section className="grid gap-3" aria-labelledby={`${form.id}-contact`}>
        <h3 id={`${form.id}-contact`} className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
          {t("contact")}
        </h3>
        {signedIn ? (
          <p className="text-sm text-muted-foreground">{t("receiptsTo", { email: checkout.email })}</p>
        ) : (
          <Field
            label={t("email")}
            field={fields.email}
            type="email"
            autoComplete="email"
            placeholder="you@example.com"
            hint={t("emailHint")}
            required
          />
        )}
        {asks.phone !== "hidden" ? (
          <Field
            label={asks.phone === "required" ? t("phone") : t("phoneOptional")}
            field={fields.phone}
            type="tel"
            phone={phoneFieldFor(props.phoneSettings, dest.countryCode)}
            autoComplete="tel"
            hint={asks.collector ? t("phoneHintPickup") : t("phoneHintDelivery")}
            required={asks.phone === "required"}
          />
        ) : null}
      </section>

      {asks.collector ? (
        <section className="grid gap-3" aria-labelledby={`${form.id}-pickup`}>
          <h3 id={`${form.id}-pickup`} className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
            {t("pickupTitle")}
          </h3>
          {props.collectFrom.map((p) => (
            <p key={p.id} className="rounded-lg border border-border p-3 text-sm">
              <span className="block font-medium">{p.name}</span>
              <span className="block text-muted-foreground">
                {[p.street1, p.city, p.postalCode].filter(Boolean).join(", ")}
              </span>
            </p>
          ))}
          {settings.pickupSomeoneElse ? (
            <fieldset className="grid gap-2 sm:grid-cols-2">
              <legend className="sr-only">{t("whoCollects")}</legend>
              <ChoiceCard name={fields.pickupWho.name} value="me" defaultChecked={!contact?.someoneElse}>
                {t("collectMe")}
              </ChoiceCard>
              <ChoiceCard name={fields.pickupWho.name} value="someoneElse" defaultChecked={!!contact?.someoneElse}>
                {t("collectSomeoneElse")}
              </ChoiceCard>
            </fieldset>
          ) : null}
          <div className="grid grid-cols-2 gap-3">
            <Field
              label={someoneElse ? t("collectorFirstName") : t("firstName")}
              field={fields.pickupFirstName}
              autoComplete={someoneElse ? "off" : "given-name"}
              required
            />
            <Field
              label={someoneElse ? t("collectorLastName") : t("lastName")}
              field={fields.pickupLastName}
              autoComplete={someoneElse ? "off" : "family-name"}
              required
            />
          </div>
          {someoneElse ? <p className="text-xs text-muted-foreground">{t("collectorHint")}</p> : null}
        </section>
      ) : null}

      {asks.shipping ? (
        <section className="grid gap-3" aria-labelledby={`${form.id}-address`}>
          <h3 id={`${form.id}-address`} className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
            {t("deliveryAddress")}
          </h3>
          <AddressChoice
            name={fields.shippingChoice.name}
            picked={typeof fields.shippingChoice.value === "string" ? fields.shippingChoice.value : shippingChoice}
            initial={shippingChoice}
            options={saved}
            errors={fields.shippingChoice.errors}
            errorId={fields.shippingChoice.errorId}
          >
            <AddressFields
              fields={shipping as never}
              countries={countries}
              regionsByCountry={regionsByCountry}
              section="shipping"
              withPhone={false}
            />
            {signedIn ? (
              <label className="mt-2 flex items-center gap-2 text-sm">
                <input type="checkbox" name={fields.saveToBook.name} className="size-4 rounded accent-primary" />
                {t("saveToBook")}
              </label>
            ) : null}
          </AddressChoice>
        </section>
      ) : null}

      {asks.shipping || asks.billing !== "hidden" || someoneElse ? (
      <section className="grid gap-3" aria-labelledby={`${form.id}-billing`}>
        <h3 id={`${form.id}-billing`} className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
          {t("billingTitle")}
        </h3>
        {!asks.shipping ? (
          billingOwed ? (
            <p className="text-xs text-muted-foreground">
              {someoneElse ? t("billingForPayer") : asks.collector ? t("billingRequired") : t("billingNoAddress")}
            </p>
          ) : (
            <Checkbox
              name={fields.addBilling.name}
              defaultChecked={fields.addBilling.initialValue === "on"}
              label={t("addBilling")}
            />
          )
        ) : (
          <Checkbox
            name={fields.billingSame.name}
            defaultChecked={fields.billingSame.initialValue === "on"}
            label={t("billingSame")}
          />
        )}
        {showBilling ? (
          <AddressChoice
            name={fields.billingChoice.name}
            picked={typeof fields.billingChoice.value === "string" ? fields.billingChoice.value : billingChoice}
            initial={billingChoice}
            options={saved}
            errors={fields.billingChoice.errors}
            errorId={fields.billingChoice.errorId}
          >
            <AddressFields
              fields={billing as never}
              countries={countries}
              regionsByCountry={regionsByCountry}
              section="billing"
              withPhone={false}
            />
          </AddressChoice>
        ) : null}
      </section>
      ) : null}

      <ContinueButton pending={pending} label={t("continueToPayment")} />
    </form>
  );
}

// Personal or business first - a business purchase names the company before anything else, then the identifiers its
// country asks for (PIB and Matični broj in Serbia, a VAT number elsewhere) and an optional PO number
function BusinessFields({
  fields,
  settings,
  business,
  idFields,
}: {
  fields: Record<"purchaseType" | "company" | "poNumber", ValueField> & {
    businessIds: { getFieldset: () => unknown };
  };
  settings: CheckoutSettings;
  business: boolean;
  idFields: BusinessIdField[];
}) {
  const t = useTranslations("checkout.details");
  const initialBusiness = fields.purchaseType.initialValue === "business";
  const ids = fields.businessIds.getFieldset() as Record<string, ValueField>;
  return (
    <section className="grid gap-3">
      <fieldset className="inline-flex w-full rounded-full border border-border p-1 text-sm sm:w-auto sm:justify-self-start">
        <legend className="sr-only">{t("purchaseType")}</legend>
        {(["personal", "business"] as const).map((v) => (
          <label
            key={v}
            className="flex-1 cursor-pointer rounded-full px-4 py-1.5 text-center font-medium transition-colors has-[:checked]:bg-foreground has-[:checked]:text-background has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-primary"
          >
            <input
              type="radio"
              name={fields.purchaseType.name}
              value={v}
              defaultChecked={(v === "business") === initialBusiness}
              className="sr-only"
            />
            {v === "business" ? t("business") : t("personal")}
          </label>
        ))}
      </fieldset>
      {business ? (
        <div className="grid gap-3 rounded-lg border border-border p-4">
          <Field label={t("company")} field={fields.company} autoComplete="organization" required />
          {idFields.map((f) => {
            const generic = f.key === GENERIC_TAX_ID && !f.label;
            const label = generic ? t("taxId") : f.label;
            return (
              <Field
                // Keyed by key so switching country swaps the inputs rather than carrying a PIB into a VAT box
                key={f.key}
                label={f.required ? label : t("optional", { label })}
                field={ids[f.key]}
                autoComplete="off"
                hint={f.hint || (generic ? t("taxIdHint") : undefined)}
                required={f.required}
              />
            );
          })}
          {settings.poNumber ? (
            <Field label={t("poNumber")} field={fields.poNumber} autoComplete="off" />
          ) : null}
        </div>
      ) : null}
    </section>
  );
}

function Checkbox({ name, defaultChecked, label }: { name: string; defaultChecked: boolean; label: string }) {
  return (
    <label className="flex items-center gap-2 text-sm">
      <input type="checkbox" name={name} defaultChecked={defaultChecked} className="size-4 rounded accent-primary" />
      {label}
    </label>
  );
}

// Each saved address as a card, then "a new address", which opens the typed-in fields. A guest with no book has
// nothing to choose, so the group is just the hidden "new" and the fields
function AddressChoice({
  name,
  picked,
  initial,
  options,
  errors,
  errorId,
  children,
}: {
  name: string;
  picked: string;
  initial: string;
  options: Address[];
  errors?: string[];
  errorId?: string;
  children: React.ReactNode;
}) {
  const t = useTranslations("checkout.details");
  if (options.length === 0) {
    return (
      <>
        <input type="hidden" name={name} value="new" />
        {children}
      </>
    );
  }
  return (
    <>
      <fieldset className="grid gap-2 sm:grid-cols-2">
        <legend className="sr-only">{t("chooseAddress")}</legend>
        {options.map((a) => (
          <ChoiceCard key={a.id} name={name} value={a.id} defaultChecked={initial === a.id}>
            <AddressLines address={a} />
          </ChoiceCard>
        ))}
        <ChoiceCard name={name} value="new" defaultChecked={initial === "new"}>
          {t("useNew")}
        </ChoiceCard>
      </fieldset>
      {errors?.length ? <FieldErrors id={errorId} errors={errors} /> : null}
      {picked === "new" ? children : null}
    </>
  );
}
