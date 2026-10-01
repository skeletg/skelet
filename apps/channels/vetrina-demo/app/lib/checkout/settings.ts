// The store's sales/checkout switches, read from publicConfig. Pure so the step logic, the schemas and their tests share
// one parse - the backend enforces the same values, so these only decide what the shopper is shown and asked

export type FieldMode = "hidden" | "optional" | "required";

export type CheckoutSettings = {
  phone: FieldMode;
  business: boolean;
  taxId: FieldMode; // only asked on a business purchase
  poNumber: boolean; // only asked on a business purchase
  pickupSomeoneElse: boolean;
  pickupRequiresBilling: boolean;
  thresholdProgress: boolean;
  termsConsent: "checkbox" | "notice" | "off";
  minOrderCents: number;
};

// The backend's registry defaults - what an unreachable or unset config reads as
export const DEFAULT_CHECKOUT_SETTINGS: CheckoutSettings = {
  phone: "optional",
  business: false,
  taxId: "optional",
  poNumber: false,
  pickupSomeoneElse: true,
  pickupRequiresBilling: false,
  thresholdProgress: true,
  termsConsent: "checkbox",
  minOrderCents: 0,
};

const fieldMode = (v: string | undefined, fallback: FieldMode): FieldMode =>
  v === "hidden" || v === "optional" || v === "required" ? v : fallback;

const flag = (v: string | undefined, fallback: boolean): boolean =>
  v === "true" ? true : v === "false" ? false : fallback;

export function parseCheckoutSettings(rows: { path: string; value: string }[]): CheckoutSettings {
  const get = (key: string) => rows.find((r) => r.path === `sales/checkout/${key}`)?.value;
  const d = DEFAULT_CHECKOUT_SETTINGS;
  const consent = get("terms_consent");
  const minOrder = Number.parseInt(get("min_order") ?? "", 10);
  return {
    phone: fieldMode(get("phone_field"), d.phone),
    business: flag(get("business_purchase"), d.business),
    taxId: fieldMode(get("tax_id_field"), d.taxId),
    poNumber: flag(get("po_number_field"), d.poNumber),
    pickupSomeoneElse: flag(get("pickup_by_someone_else"), d.pickupSomeoneElse),
    pickupRequiresBilling: flag(get("pickup_requires_billing_address"), d.pickupRequiresBilling),
    thresholdProgress: flag(get("threshold_progress"), d.thresholdProgress),
    termsConsent: consent === "checkbox" || consent === "notice" || consent === "off" ? consent : d.termsConsent,
    minOrderCents: Number.isFinite(minOrder) && minOrder > 0 ? minOrder : 0,
  };
}

// A pickup has no shipping address to name the payer, so a billing address is owed when the store wants one on every
// invoice or someone else collects - the same rule the backend's detailsComplete applies
export function pickupNeedsBilling(s: CheckoutSettings, someoneElse: boolean): boolean {
  return s.pickupRequiresBilling || someoneElse;
}
