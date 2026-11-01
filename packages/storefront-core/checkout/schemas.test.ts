import { describe, expect, it } from "vitest";

import type { DetailsAsks } from "./asks";
import {
  groupDeliverySchema,
  destinationSchema,
  detailsStepSchema,
  paymentStepSchema,
  type DetailsMode,
  splitSelection,
} from "./schemas";
import { DEFAULT_CHECKOUT_SETTINGS as defaults } from "./settings";
import type { CountryOption } from "../countries/read";
import { parseForm } from "../forms/schema";

// Field errors of a failed parse, undefined when it succeeded
function errorsOf(s: object): Record<string, string[] | null> | undefined {
  return "error" in s ? ((s.error as Record<string, string[] | null> | null) ?? undefined) : undefined;
}

function form(entries: [string, string][]): FormData {
  const fd = new FormData();
  for (const [k, v] of entries) fd.append(k, v);
  return fd;
}

const typed = (prefix: string): [string, string][] => [
  [`${prefix}.firstName`, "Ana"],
  [`${prefix}.lastName`, "Petrovic"],
  [`${prefix}.street1`, "Knez Mihailova 1"],
  [`${prefix}.city`, "Belgrade"],
  [`${prefix}.postalCode`, "11000"],
  [`${prefix}.countryCode`, "rs"],
];

// Serbia defines its own company identifiers; the US leaves them to the generic tax ID
const countries: CountryOption[] = [
  {
    code: "RS",
    name: "Serbia",
    emojiFlag: "",
    businessIdFields: [
      { key: "pib", label: "PIB", required: true, pattern: "\\d{9}", hint: "" },
      { key: "mb", label: "Matični broj", required: false, pattern: "^\\d{8}$", hint: "" },
    ],
  },
  { code: "US", name: "United States", emojiFlag: "", businessIdFields: [] },
];
// What a courier delivery asks, and what a store pickup does
const DELIVERY: DetailsAsks = { shipping: true, collector: false, billing: "required", phone: "optional" };
const PICKUP: DetailsAsks = { shipping: false, collector: true, billing: "optional", phone: "optional" };
const LOCKER: DetailsAsks = { shipping: false, collector: false, billing: "required", phone: "required" };
const mode = (m: Partial<DetailsMode> = {}): DetailsMode => ({
  asks: DELIVERY,
  signedIn: false,
  countries,
  savedCountries: { "addr-1": "RS" },
  destinationCountry: "RS",
  ...m,
});
const guestDelivery = detailsStepSchema(defaults, mode());
const guestPickup = detailsStepSchema(defaults, mode({ asks: PICKUP }));

describe("destinationSchema", () => {
  it("needs a country and a postcode, and uppercases the country", () => {
    expect(errorsOf(parseForm(form([["countryCode", "RS"]]), destinationSchema))?.["postalCode"]).toEqual([
      "postalCodeRequired",
    ]);
    const s = parseForm(form([["countryCode", "rs"], ["postalCode", " 21000 "]]), destinationSchema);
    expect(s.status === "success" && s.value).toEqual({ countryCode: "RS", postalCode: "21000" });
  });
});

describe("detailsStepSchema - delivery", () => {
  it("takes a saved address by id alone", () => {
    const s = parseForm(
      form([["email", "a@b.rs"], ["shippingChoice", "addr-1"], ["billingSame", "on"]]),
      guestDelivery,
    );
    expect(s.status).toBe("success");
  });

  it("requires the address fields only for a new address, under their nested names", () => {
    const s = parseForm(
      form([["email", "a@b.rs"], ["shippingChoice", "new"], ["shipping.firstName", "Ana"], ["billingSame", "on"]]),
      guestDelivery,
    );
    expect(errorsOf(s)?.["shipping.street1"]).toEqual(["required"]);
    expect(errorsOf(s)?.["shipping.firstName"]).toBeUndefined();
  });

  it("asks for billing only when the same-as-delivery box is cleared", () => {
    const base: [string, string][] = [["email", "a@b.rs"], ["shippingChoice", "new"], ...typed("shipping")];
    expect(parseForm(form([...base, ["billingSame", "on"]]), guestDelivery).status).toBe("success");
    expect(errorsOf(parseForm(form(base), guestDelivery))?.["billingChoice"]).toEqual(["chooseAddress"]);
    expect(parseForm(form([...base, ["billingChoice", "new"], ...typed("billing")]), guestDelivery).status).toBe(
      "success",
    );
  });

  it("reports every missing field on the first submit, not the email alone", () => {
    const biz = detailsStepSchema({ ...defaults, business: true }, mode({ signedIn: false }));
    const errors = errorsOf(parseForm(form([["shippingChoice", "new"], ["purchaseType", "business"]]), biz));
    expect(errors?.["email"]).toEqual(["required"]);
    expect(errors?.["company"]).toEqual(["companyRequired"]);
    expect(errors?.["shipping.street1"]).toEqual(["required"]);
    expect(errors?.["billingChoice"]).toEqual(["chooseAddress"]);
  });

  it("wants an email from a guest but not from a signed-in customer", () => {
    const entries: [string, string][] = [["shippingChoice", "addr-1"], ["billingSame", "on"]];
    expect(errorsOf(parseForm(form(entries), guestDelivery))?.["email"]).toBeDefined();
    expect(parseForm(form(entries), detailsStepSchema(defaults, mode({ signedIn: true }))).status).toBe(
      "success",
    );
  });
});

describe("detailsStepSchema - pickup", () => {
  it("needs who collects and no address", () => {
    expect(errorsOf(parseForm(form([["email", "a@b.rs"]]), guestPickup))?.["pickupFirstName"]).toEqual(["required"]);
    const s = parseForm(
      form([["email", "a@b.rs"], ["pickupFirstName", "Ana"], ["pickupLastName", "P"], ["pickupWho", "me"]]),
      guestPickup,
    );
    expect(s.status).toBe("success");
  });

  it("asks for the payer's billing address when someone else collects", () => {
    const entries: [string, string][] = [
      ["email", "a@b.rs"],
      ["pickupFirstName", "Ana"],
      ["pickupLastName", "P"],
      ["pickupWho", "someoneElse"],
    ];
    expect(errorsOf(parseForm(form(entries), guestPickup))?.["billingChoice"]).toEqual(["chooseAddress"]);
    // With the setting off there's no "someone else" to choose, so nothing more is owed
    const noProxy = detailsStepSchema({ ...defaults, pickupSomeoneElse: false }, mode({ asks: PICKUP, signedIn: false }));
    expect(parseForm(form(entries), noProxy).status).toBe("success");
  });
});

describe("detailsStepSchema - PO box", () => {
  const locker = detailsStepSchema(defaults, mode({ asks: LOCKER }));

  it("asks no address of the shopper and no collector, but billing and the phone the delivery needs", () => {
    const errors = errorsOf(parseForm(form([["email", "a@b.rs"]]), locker));
    expect(errors?.["shippingChoice"]).toBeUndefined();
    expect(errors?.["pickupFirstName"]).toBeUndefined();
    expect(errors?.["billingChoice"]).toEqual(["chooseAddress"]);
    expect(errors?.["phone"]).toEqual(["phoneRequired"]);
    const ok = parseForm(
      form([["email", "a@b.rs"], ["phone", "064 1234567"], ["phoneCountry", "RS"], ["billingChoice", "new"], ...typed("billing")]),
      locker,
    );
    expect(ok.status).toBe("success");
  });
});

describe("detailsStepSchema - settings", () => {
  const entries: [string, string][] = [["email", "a@b.rs"], ["shippingChoice", "addr-1"], ["billingSame", "on"]];

  it("requires the phone only when the store does, and checks it against the destination's country", () => {
    const required = detailsStepSchema(defaults, mode({ asks: { ...DELIVERY, phone: "required" }, signedIn: false }));
    expect(errorsOf(parseForm(form(entries), required))?.["phone"]).toEqual(["phoneRequired"]);
    const bad = parseForm(form([...entries, ["phone", "123"], ["phoneCountry", "RS"]]), guestDelivery);
    expect(errorsOf(bad)?.["phone"]).toEqual(["phoneInvalid"]);
    const ok = parseForm(form([...entries, ["phone", "064 1234567"], ["phoneCountry", "RS"]]), guestDelivery);
    expect(ok.status).toBe("success");
  });

  it("keeps the phone to the store's countries when it says so", () => {
    const serbiaOnly = [...entries, ["phoneCountry", "RS"], ["phoneAllowed", "RS"]] as typeof entries;
    const foreign = parseForm(form([...serbiaOnly, ["phone", "+49 1512 3456789"]]), guestDelivery);
    expect(errorsOf(foreign)?.["phone"]).toEqual(["phoneCountryNotAllowed"]);
    const home = parseForm(form([...serbiaOnly, ["phone", "064 1234567"]]), guestDelivery);
    expect(home.status).toBe("success");
  });

  it("asks a business buyer for the company and the identifiers the company's country defines", () => {
    const biz = detailsStepSchema({ ...defaults, business: true }, mode());
    // Serbia: the PIB is required and must be nine digits, the Matični broj is optional but eight digits when given
    const s = parseForm(form([...entries, ["purchaseType", "business"], ["businessIds.mb", "123"]]), biz);
    expect(errorsOf(s)?.["company"]).toEqual(["companyRequired"]);
    expect(errorsOf(s)?.["businessIds.pib"]).toEqual(["required"]);
    expect(errorsOf(s)?.["businessIds.mb"]).toEqual(["businessIdInvalid"]);
    const ok = parseForm(
      form([...entries, ["purchaseType", "business"], ["company", "Acme"], ["businessIds.pib", "123456789"]]),
      biz,
    );
    expect(ok.status === "success" && ok.value.businessIds).toEqual({ pib: "123456789" });
  });

  it("falls back to the generic tax ID where the country defines none, as the setting says", () => {
    const us = mode({ destinationCountry: "US", savedCountries: { "addr-1": "US" } });
    const required = detailsStepSchema({ ...defaults, business: true, taxId: "required" }, us);
    const s = parseForm(form([...entries, ["purchaseType", "business"], ["company", "Acme"]]), required);
    expect(errorsOf(s)?.["businessIds.tax_id"]).toEqual(["required"]);
    const hidden = detailsStepSchema({ ...defaults, business: true, taxId: "hidden" }, us);
    expect(parseForm(form([...entries, ["purchaseType", "business"], ["company", "Acme"]]), hidden).status).toBe("success");
  });

  it("follows the billing address's country when billing is separate", () => {
    const biz = detailsStepSchema({ ...defaults, business: true }, mode());
    // Delivered to Serbia, billed to a US company - the PIB isn't owed
    const s = parseForm(
      form([
        ["email", "a@b.rs"],
        ["shippingChoice", "addr-1"],
        ["billingChoice", "new"],
        ...typed("billing").map(([k, v]): [string, string] => (k.endsWith("countryCode") ? [k, "us"] : [k, v])),
        ["purchaseType", "business"],
        ["company", "Acme"],
      ]),
      biz,
    );
    expect(s.status).toBe("success");
    // Business purchases off - the switch posting "business" asks for nothing
    expect(parseForm(form([...entries, ["purchaseType", "business"]]), guestDelivery).status).toBe("success");
  });
});

describe("groupDeliverySchema", () => {
  it("needs an option::carrier::method selection for a group, and the method may contain colons", () => {
    expect(errorsOf(parseForm(form([["groupKey", "all"]]), groupDeliverySchema))?.["selection"]).toEqual(["shippingMethodRequired"]);
    const s = parseForm(form([["groupKey", "all"], ["selection", "opt-1::ups::ups:03"]]), groupDeliverySchema);
    expect(s.status === "success" && splitSelection(s.value.selection)).toEqual({
      optionId: "opt-1",
      carrierSlug: "ups",
      methodCode: "ups:03",
    });
  });
});

describe("paymentStepSchema", () => {
  const checkbox = paymentStepSchema({ requireMethod: true, consent: "checkbox" });

  it("refuses to place without the terms when the store asks for the box", () => {
    const s = parseForm(form([["providerSlug", "cod"]]), checkbox);
    expect(errorsOf(s)?.["tos"]).toEqual(["tosRequired"]);
    const notice = paymentStepSchema({ requireMethod: true, consent: "notice" });
    expect(parseForm(form([["providerSlug", "cod"]]), notice).status).toBe("success");
  });

  it("needs no method when gift cards or store credit cover the total", () => {
    const covered = paymentStepSchema({ requireMethod: false, consent: "checkbox" });
    expect(parseForm(form([["tos", "on"]]), covered).status).toBe("success");
    expect(errorsOf(parseForm(form([["tos", "on"]]), checkbox))?.["providerSlug"]).toEqual(["paymentMethodRequired"]);
  });

  it("reads blanks as null", () => {
    const s = parseForm(form([["providerSlug", "cod"], ["tos", "on"], ["note", " "], ["savedPaymentMethodId", ""]]), checkbox);
    expect(s.status === "success" && s.value).toEqual({
      providerSlug: "cod",
      tos: true,
      note: null,
      savedPaymentMethodId: null,
    });
  });
});
