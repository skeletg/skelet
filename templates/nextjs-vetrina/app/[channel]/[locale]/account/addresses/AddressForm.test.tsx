import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { addressSchema } from "@skelet/storefront-core/auth/schemas";
import { parseForm, type FormResult } from "@skelet/storefront-core/forms/schema";
import { localeLinkMock } from "@/app/test/locale-link-mock";
import { renderWithIntl } from "@/app/test/render";

vi.mock("@/app/components/LocaleLink", () => localeLinkMock);

// A backend that refuses every address, so the test sees what a shopper sees when the save fails
const addAddressAction = vi.fn(async (_prev: FormResult | undefined, formData: FormData) => {
  const submission = parseForm(formData, addressSchema);
  if (submission.status !== "success") return submission.reply();
  return submission.reply({ formErrors: ["Postal code does not match the city."] });
});
vi.mock("@skelet/storefront-next/auth/actions", () => ({
  addAddressAction: (prev: FormResult | undefined, fd: FormData) => addAddressAction(prev, fd),
  updateAddressAction: async () => ({}),
}));

const { AddressForm } = await import("@/app/[channel]/[locale]/account/addresses/AddressForm");

// The store sells to two countries - Germany with seeded regions, the US with the built-in state list
const options = {
  countries: [
    { code: "DE", name: "Germany", emojiFlag: "" },
    { code: "US", name: "United States", emojiFlag: "" },
  ],
  regionsByCountry: { DE: [{ code: "DE-BE", name: "Berlin" }, { code: "DE-BY", name: "Bavaria" }] },
  // No store phone policy - numbers are read in the address's own country
  phone: { restrictedToStoreCountries: false, lockedCountryCode: null, defaultCountryCode: "DE", formats: [] },
  defaultCountry: "DE",
};

async function fillRequired(user: ReturnType<typeof userEvent.setup>, country: string) {
  await user.type(screen.getByLabelText("First name"), "Ana");
  await user.type(screen.getByLabelText("Last name"), "Müller");
  await user.type(screen.getByLabelText("Street address"), "Main Street 1");
  await user.type(screen.getByLabelText("City"), "Berlin");
  await user.type(screen.getByLabelText("Postal code"), "10115");
  await user.selectOptions(screen.getByLabelText("Country"), country);
}

describe("AddressForm", () => {
  it("offers only the store's countries, starting a new address on its default", () => {
    renderWithIntl(<AddressForm submitLabel="Save address" options={options} />);
    const country = screen.getByLabelText("Country");
    expect(country).toHaveValue("DE");
    const offered = Array.from((country as HTMLSelectElement).options).map((o) => o.value).filter(Boolean);
    expect(offered).toEqual(["DE", "US"]);
  });

  it("picks the region from the country's own list, and swaps the list with the country", async () => {
    const user = userEvent.setup();
    renderWithIntl(<AddressForm submitLabel="Save address" options={options} />);
    const regions = () =>
      Array.from((screen.getByLabelText("State / region") as HTMLSelectElement).options).map((o) => o.textContent);
    expect(regions()).toEqual(["Select…", "Berlin", "Bavaria"]);

    await user.selectOptions(screen.getByLabelText("Country"), "US");
    expect(regions()).toContain("California");
    expect(regions()).not.toContain("Berlin");
  });

  it("opens an existing address on its own country and region", () => {
    renderWithIntl(
      <AddressForm
        id="a1"
        submitLabel="Save"
        options={options}
        initial={{ firstName: "Ana", countryCode: "US", state: "CA" }}
      />,
    );
    expect(screen.getByLabelText("Country")).toHaveValue("US");
    expect(screen.getByLabelText("State / region")).toHaveValue("CA");
  });

  it("keeps every typed value and shows the backend's reason when the save fails", async () => {
    const user = userEvent.setup();
    renderWithIntl(<AddressForm submitLabel="Save address" options={options} />);
    await fillRequired(user, "DE");
    await user.type(screen.getByLabelText("Phone (optional)"), "+49 30 12345678");
    await user.click(screen.getByRole("button", { name: "Save address" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("Postal code does not match the city.");
    await waitFor(() => expect(screen.getByLabelText("City")).toHaveValue("Berlin"));
    expect(screen.getByLabelText("Street address")).toHaveValue("Main Street 1");
    expect(screen.getByLabelText("Phone (optional)")).toHaveValue("+49 30 12345678");
  });

  it("formats a phone as it's typed, in the address's country", async () => {
    const user = userEvent.setup();
    renderWithIntl(<AddressForm submitLabel="Save address" options={options} />);
    await fillRequired(user, "DE");
    await user.type(screen.getByLabelText("Phone (optional)"), "015123456789");
    expect(screen.getByLabelText("Phone (optional)")).toHaveValue("01512 3456789");
  });

  it("refuses a phone that can't be a number in that country before posting", async () => {
    addAddressAction.mockClear();
    const user = userEvent.setup();
    renderWithIntl(<AddressForm submitLabel="Save address" options={options} />);
    await fillRequired(user, "DE");
    await user.type(screen.getByLabelText("Phone (optional)"), "0151");
    await user.click(screen.getByRole("button", { name: "Save address" }));

    expect(screen.getByLabelText("Phone (optional)")).toHaveAccessibleDescription(
      "Enter a valid phone number, or start it with + and the country code.",
    );
    expect(addAddressAction).not.toHaveBeenCalled();
  });
});
