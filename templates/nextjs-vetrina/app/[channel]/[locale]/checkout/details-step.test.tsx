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
    lastName: "Ilić",
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
      me={signedIn ? { firstName: "Ana", lastName: "Ilić" } : null}
      saved={props.saved ?? [HOME, OFFICE]}
      defaultShippingId={props.defaultShippingId ?? null}
      defaultBillingId={props.defaultBillingId ?? null}
      countries={[{ code: "US", name: "United States", emojiFlag: null } as never]}
      regionsByCountry={{}}
      phoneSettings={props.phoneSettings ?? ANY_PHONE}
      asks={{ shipping: true, collector: false, billing: "required", phone: "optional" }}
      collectFrom={[]}
      locale="en"
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
        lockedCountryCode: "RS",
        defaultCountryCode: "RS",
        formats: [{ countryCode: "RS", callingCode: "+381", example: "+381 60 1234567", nationalExample: "060 1234567", template: "xxx xxxxxxx" }],
      },
    });
    expect(screen.getByText("+381")).toBeInTheDocument();
    expect(container.querySelector('input[type="tel"]')).toHaveAttribute("placeholder", "060 1234567");
    expect(container.querySelector<HTMLInputElement>('input[name="phoneCountry"]')?.value).toBe("RS");
    expect(container.querySelector<HTMLInputElement>('input[name="phoneAllowed"]')?.value).toBe("RS");
  });
});
