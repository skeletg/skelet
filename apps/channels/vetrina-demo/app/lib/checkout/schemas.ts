import { z } from "zod";

import { addressFields, addressSchema } from "@/app/lib/auth/schemas";
import { emailField, optionalText, requiredText } from "@/app/lib/forms/schema";
import { refinePhone } from "@/app/lib/forms/phone";

import type { CountryOption } from "@/app/lib/countries/read";

import { businessIdFieldsFor, companyCountry, matchesPattern } from "./business-ids";
import { pickupNeedsBilling, type CheckoutSettings } from "./settings";

// One schema per checkout step, shared by the step's form (client validation) and its action. Messages are
// forms.errors keys, like every other storefront form

// Step 1 - where the order is going. A postcode is all delivery and pickup need to be quoted
export const destinationSchema = z.object({
  countryCode: requiredText()
    .toUpperCase()
    .regex(/^[A-Z]{2}$/, "countryCode"),
  postalCode: z.string({ error: "postalCodeRequired" }).trim().min(1, "postalCodeRequired").max(20, "tooLong"),
});

// selection is "<carrierSlug>::<code>", one radio per quote
export const deliveryStepSchema = z.object({
  selection: z
    .string({ error: "shippingMethodRequired" })
    .regex(/^[^:]+::.+$/, "shippingMethodRequired"),
});

export function splitSelection(selection: string): { carrierSlug: string; code: string } {
  const at = selection.indexOf("::");
  return { carrierSlug: selection.slice(0, at), code: selection.slice(at + 2) };
}

// Every address field optional at the shape level - which ones are required depends on the choices, so the address
// schema is re-run in the refinement only when the shopper is typing one in. The contact phone is asked once, above
const looseAddress = addressFields.omit({ phone: true }).partial().optional();

export type DetailsMode = {
  pickup: boolean;
  signedIn: boolean;
  // What the company identifiers are judged against - the store's countries with their fields, the saved addresses'
  // countries by id, and where the order goes
  countries: CountryOption[];
  savedCountries: Record<string, string>;
  destinationCountry: string;
};

type LooseValues = {
  shippingChoice?: string | null;
  shipping?: { countryCode?: string | null } | null;
  billingChoice?: string | null;
  billing?: { countryCode?: string | null } | null;
  billingSame?: boolean;
};

// The company's country as the form currently has it - a billing address when one is given, else the delivery address
// (a pickup has none), else the destination. The same reading the page makes of the saved cart
export function detailsCompanyCountry(v: LooseValues, mode: DetailsMode, billingShown: boolean): string {
  const of = (choice: string | null | undefined, addr: { countryCode?: string | null } | null | undefined) =>
    choice === "new" ? (addr?.countryCode ?? "").toUpperCase() : choice ? (mode.savedCountries[choice] ?? "") : "";
  return companyCountry({
    billingCountry: billingShown ? of(v.billingChoice, v.billing) : null,
    deliveryCountry: mode.pickup ? null : of(v.shippingChoice, v.shipping),
    destinationCountry: mode.destinationCountry,
  });
}

// Step 2 - contact, business details, then the delivery address or who collects a pickup, and billing. What is asked
// and required follows the store's settings; the backend applies the same rules
//
// Every field is optional at the shape level and the requirements live in the refinement: zod skips an object's
// refinements once a field fails its type, so a missing email would otherwise hide every other error until the second
// submit
//
// shippingChoice and billingChoice are a saved address id or "new". billingSame is the delivery's "billing address is
// the same" box; a pickup has no address to mirror, so addBilling opts into one when it isn't required
export function detailsStepSchema(s: CheckoutSettings, mode: DetailsMode) {
  return z
    .object({
      email: optionalText(),
      phone: optionalText(),
      // PhoneInput's hidden companions (refinePhone) - the country a nationally typed number is read in, as the backend
      // reads it, and the store's countries when it keeps phones to them
      phoneCountry: optionalText(),
      phoneAllowed: optionalText(),
      purchaseType: z.enum(["personal", "business"]).optional(),
      company: optionalText(200),
      // By the field key of the company's country ("businessIds.pib")
      businessIds: z.record(z.string(), optionalText(100)).optional(),
      poNumber: optionalText(100),
      shippingChoice: optionalText(),
      shipping: looseAddress,
      saveToBook: z.boolean().optional(),
      billingSame: z.boolean().optional(),
      addBilling: z.boolean().optional(),
      billingChoice: optionalText(),
      billing: looseAddress,
      pickupWho: z.enum(["me", "someoneElse"]).optional(),
      pickupFirstName: optionalText(100),
      pickupLastName: optionalText(100),
    })
    .superRefine((v, ctx) => {
      const issue = (path: (string | number)[], message: string) => ctx.addIssue({ code: "custom", path, message });
      if (!mode.signedIn && !emailField().safeParse(v.email ?? undefined).success) issue(["email"], v.email ? "email" : "required");
      if (s.phone === "required" && !v.phone) issue(["phone"], "phoneRequired");
      refinePhone("phone")(v, ctx);

      const someoneElse = s.pickupSomeoneElse && v.pickupWho === "someoneElse";
      const billingShown = mode.pickup ? pickupNeedsBilling(s, someoneElse) || !!v.addBilling : !v.billingSame;
      if (s.business && v.purchaseType === "business") {
        if (!v.company) issue(["company"], "companyRequired");
        const country = detailsCompanyCountry(v, mode, billingShown);
        for (const f of businessIdFieldsFor(mode.countries, country, s)) {
          const value = v.businessIds?.[f.key] ?? "";
          if (!value && f.required) issue(["businessIds", f.key], "required");
          else if (value && !matchesPattern(f.pattern, value)) issue(["businessIds", f.key], "businessIdInvalid");
        }
      }

      const address = (key: "shipping" | "billing", choice: string | null) => {
        if (!choice) {
          issue([`${key}Choice`], "chooseAddress");
          return;
        }
        if (choice !== "new") return;
        const parsed = addressSchema.safeParse(v[key] ?? {});
        if (parsed.success) return;
        for (const i of parsed.error.issues) issue([key, ...i.path.map(String)], i.message);
      };

      if (mode.pickup) {
        if (!v.pickupFirstName) issue(["pickupFirstName"], "required");
        if (!v.pickupLastName) issue(["pickupLastName"], "required");
        if (billingShown) address("billing", v.billingChoice);
        return;
      }
      address("shipping", v.shippingChoice);
      if (billingShown) address("billing", v.billingChoice);
    });
}

export type DetailsValues = z.output<ReturnType<typeof detailsStepSchema>>;

export type PaymentMode = { requireMethod: boolean; consent: CheckoutSettings["termsConsent"] };

// The last step - submitting it saves the payment choice and the note, then places the order. A total covered by gift
// cards or store credit needs no method; the terms box goes to placeOrder, which refuses an unticked one when the store asks for it
export function paymentStepSchema(mode: PaymentMode) {
  return z.object({
    providerSlug: mode.requireMethod
      ? z.string({ error: "paymentMethodRequired" }).min(1, "paymentMethodRequired")
      : optionalText(),
    savedPaymentMethodId: optionalText(),
    // The cart note allows 2000, and checkout carries the same note
    note: optionalText(2000),
    tos:
      mode.consent === "checkbox"
        ? z.boolean({ error: "tosRequired" }).refine((v) => v, "tosRequired")
        : z.boolean().optional(),
  });
}

export const giftCardSchema = z.object({
  code: z.string({ error: "giftCardRequired" }).trim().min(1, "giftCardRequired"),
});
