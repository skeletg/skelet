import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import type { CheckoutAddress, CheckoutState } from "@skelet/storefront-next/checkout/read";
import { DEFAULT_CHECKOUT_SETTINGS } from "@skelet/storefront-core/checkout/settings";
import type { PhoneSettings } from "@skelet/storefront-next/countries/read";
import type { Address } from "@skelet/storefront-next/customer/read";
import { renderWithIntl } from "@/app/test/render";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: vi.fn() }),
  usePathname: () => "/checkout",
  useSearchParams: () => new URLSearchParams(),
}));
vi.mock("@skelet/storefront-next/checkout/actions", () => ({ saveDetailsStep: async () => ({}) }));

const { DetailsStep } = await import("@/app/[channel]/[locale]/checkout/details-step");

function address(id: string, street1: string): Address {
  return {
    id,
    firstName: "Ana",
    lastName: "Müller",
    company: null,
    street1,
    street2: null,
    city: "New York",
    state: "NY",
    postalCode: "10118",
    countryCode: "US",
    phone: null,
  };
}

const HOME = address("a-home", "1 Home Street");
const OFFICE = address("a-office", "2 Office Avenue");

// A cart that's past the delivery step and has no details saved yet
function checkout(over: Partial<CheckoutState> = {}): CheckoutState {
  return {
    email: "ana@example.com",
    destination: { countryCode: "US", regionCode: "NY", postalCode: "10118" },
    shippingMethod: null,
    isPickup: false,
    phone: "",
    purchaseType: "PERSONAL",
    company: "",
    businessIds: [],
    poNumber: "",
    fields: [],
    shippingAddress: null,
    billingSameAsShipping: true,
    billingAddress: null,
    pickupContact: null,
    paymentProviderSlug: null,
    giftCards: [],
    useStoreCredit: false,
    totals: {
      subtotalCents: 0,
      discountCents: 0,
      shippingCents: 0,
      taxCents: 0,
      totalCents: 0,
      giftCardCents: 0,
      storeCreditCents: 0,
      amountDueCents: 0,
    },
    ...over,
  } as CheckoutState;
}

function snapshot(a: Address, sourceAddressId: string | null): CheckoutAddress {
  const { id: _id, ...rest } = a;
  return { ...rest, sourceAddressId } as CheckoutAddress;
}

// The default policy: any number, read in the destination's country
const ANY_PHONE: PhoneSettings = { restrictedToStoreCountries: false, lockedCountryCode: null, defaultCountryCode: "US", formats: [] };

function renderStep(props: {
  checkout?: CheckoutState;
  saved?: Address[];
  defaultShippingId?: string | null;
  defaultBillingId?: string | null;
  signedIn?: boolean;
  phoneSettings?: PhoneSettings;
}) {
  const signedIn = props.signedIn ?? true;
  return renderWithIntl(
    <DetailsStep
      checkout={props.checkout ?? checkout()}
      settings={DEFAULT_CHECKOUT_SETTINGS}
      signedIn={signedIn}
      me={signedIn ? { firstName: "Ana", lastName: "Müller" } : null}
      saved={props.saved ?? [HOME, OFFICE]}
      defaultShippingId={props.defaultShippingId ?? null}
      defaultBillingId={props.defaultBillingId ?? null}
      countries={[{ code: "US", name: "United States", emojiFlag: null } as never]}
      regionsByCountry={{}}
      phoneSettings={props.phoneSettings ?? ANY_PHONE}
      asks={{ shipping: true, collector: false, billing: "required", phone: "optional" }}
      collectFrom={[]}
    />,
  );
}

const delivery = () => within(screen.getByRole("region", { name: "Delivery address" }));
const billing = () => within(screen.getByRole("region", { name: "Billing address" }));

describe("DetailsStep address picker", () => {
  it("preselects the default delivery address", () => {
    renderStep({ defaultShippingId: OFFICE.id });
    expect(delivery().getByRole("radio", { name: /2 Office Avenue/ })).toBeChecked();
    expect(delivery().queryByLabelText("Street address")).toBeNull();
  });

  it("falls back to the first saved address when there's no default", () => {
    renderStep({});
    expect(delivery().getByRole("radio", { name: /1 Home Street/ })).toBeChecked();
  });

  it("keeps the saved address the cart already holds over the default", () => {
    renderStep({
      checkout: checkout({ shippingAddress: snapshot(HOME, HOME.id) }),
      defaultShippingId: OFFICE.id,
    });
    expect(delivery().getByRole("radio", { name: /1 Home Street/ })).toBeChecked();
  });

  it("gives a guest the typed-in fields and nothing to choose", () => {
    renderStep({ saved: [], signedIn: false });
    expect(delivery().queryByRole("radio")).toBeNull();
    expect(delivery().getByLabelText("Street address")).toBeInTheDocument();
  });

  it("opens the fields once a new address is picked", async () => {
    const user = userEvent.setup();
    renderStep({ defaultShippingId: HOME.id });
    await user.click(delivery().getByRole("radio", { name: "Use a new address" }));
    expect(delivery().getByLabelText("Street address")).toBeInTheDocument();
  });

  it("opens billing on the default billing address when it isn't the delivery one", () => {
    renderStep({ defaultShippingId: HOME.id, defaultBillingId: OFFICE.id });
    expect(billing().getByRole("checkbox", { name: "Same as the delivery address" })).not.toBeChecked();
    expect(billing().getByRole("radio", { name: /2 Office Avenue/ })).toBeChecked();
  });

  it("keeps billing the same as delivery when the defaults match", () => {
    renderStep({ defaultShippingId: HOME.id, defaultBillingId: HOME.id });
    expect(billing().getByRole("checkbox", { name: "Same as the delivery address" })).toBeChecked();
    expect(billing().queryByRole("radio")).toBeNull();
  });

  it("keeps what a saved step chose over the defaults", () => {
    renderStep({
      checkout: checkout({ shippingAddress: snapshot(HOME, HOME.id), billingSameAsShipping: true }),
      defaultShippingId: HOME.id,
      defaultBillingId: OFFICE.id,
    });
    expect(billing().getByRole("checkbox", { name: "Same as the delivery address" })).toBeChecked();
  });
});

describe("DetailsStep contact phone", () => {
  it("reads the phone in the destination's country by default, with no fixed prefix", () => {
    const { container } = renderStep({});
    const country = container.querySelector<HTMLInputElement>('input[name="phoneCountry"]');
    expect(country?.value).toBe("US");
    expect(container.querySelector('input[name="phoneAllowed"]')).toBeNull();
    expect(screen.queryByText("+1")).toBeNull();
  });

  it("fixes a single-country store's prefix and keeps the phone to it", () => {
    const { container } = renderStep({
      phoneSettings: {
        restrictedToStoreCountries: true,
        lockedCountryCode: "DE",
        defaultCountryCode: "DE",
        formats: [{ countryCode: "DE", callingCode: "+49", example: "+49 1512 3456789", nationalExample: "01512 3456789", template: "xxxxx xxxxxxx" }],
      },
    });
    expect(screen.getByText("+49")).toBeInTheDocument();
    expect(container.querySelector('input[type="tel"]')).toHaveAttribute("placeholder", "01512 3456789");
    expect(container.querySelector<HTMLInputElement>('input[name="phoneCountry"]')?.value).toBe("DE");
    expect(container.querySelector<HTMLInputElement>('input[name="phoneAllowed"]')?.value).toBe("DE");
  });
});

describe("DetailsStep plugin fields", () => {
  const spec = { hint: "", options: [], required: false, maxLength: 200, value: "" };

  it("asks what the store's plugins ask, each the way its type says, with the answer saved before", () => {
    renderStep({
      checkout: checkout({
        fields: [
          { ...spec, key: "acme:gift_message", label: "Gift message", type: "TEXTAREA", value: "Happy birthday" },
          { ...spec, key: "acme:wrap", label: "Wrapping", type: "SELECT", options: [{ value: "paper", label: "Paper" }, { value: "box", label: "Box" }], required: true },
          { ...spec, key: "acme:terms", label: "Accept the gift terms", type: "CHECKBOX", required: true, value: "true" },
          { ...spec, key: "acme:day", label: "Delivery day", type: "DATE", hint: "We deliver on weekdays" },
        ],
      }),
    });
    const more = within(screen.getByRole("region", { name: "A few more details" }));
    expect(more.getByRole("textbox", { name: "Gift message (optional)" })).toHaveValue("Happy birthday");
    expect(more.getByRole("combobox", { name: "Wrapping" })).toHaveValue("");
    expect(more.getByRole("checkbox", { name: "Accept the gift terms" })).toBeChecked();
    expect(more.getByLabelText("Delivery day (optional)")).toHaveAttribute("type", "date");
    expect(more.getByText("We deliver on weekdays")).toBeInTheDocument();
  });

  it("asks nothing extra when no plugin does", () => {
    renderStep({});
    expect(screen.queryByRole("region", { name: "A few more details" })).toBeNull();
  });
});
