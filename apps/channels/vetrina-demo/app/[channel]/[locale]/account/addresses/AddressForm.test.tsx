import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { addressSchema } from "@/app/lib/auth/schemas";
import { parseForm, type FormResult } from "@/app/lib/forms/schema";
import { localeLinkMock } from "@/app/test/locale-link-mock";
import { renderWithIntl } from "@/app/test/render";

vi.mock("@/app/components/LocaleLink", () => localeLinkMock);

// A backend that refuses every address, so the test sees what a shopper sees when the save fails
const addAddressAction = vi.fn(async (_prev: FormResult | undefined, formData: FormData) => {
  const submission = parseForm(formData, addressSchema);
  if (submission.status !== "success") return submission.reply();
  return submission.reply({ formErrors: ["Postal code does not match the city."] });
});
vi.mock("@/app/lib/auth/actions", () => ({
  addAddressAction: (prev: FormResult | undefined, fd: FormData) => addAddressAction(prev, fd),
  updateAddressAction: async () => ({}),
}));

const { AddressForm } = await import("@/app/[channel]/[locale]/account/addresses/AddressForm");

async function fillRequired(user: ReturnType<typeof userEvent.setup>, country: string) {
  await user.type(screen.getByLabelText("First name"), "Ana");
  await user.type(screen.getByLabelText("Last name"), "Ilić");
  await user.type(screen.getByLabelText("Street address"), "Knez Mihailova 1");
  await user.type(screen.getByLabelText("City"), "Beograd");
  await user.type(screen.getByLabelText("Postal code"), "11000");
  await user.type(screen.getByLabelText("Country code (2-letter)"), country);
}

describe("AddressForm", () => {
  it("rejects a country that isn't a 2-letter code before posting", async () => {
    const user = userEvent.setup();
    renderWithIntl(<AddressForm submitLabel="Save address" />);
    await fillRequired(user, "SR");
    await user.clear(screen.getByLabelText("Country code (2-letter)"));
    await user.type(screen.getByLabelText("Country code (2-letter)"), "S1");
    await user.click(screen.getByRole("button", { name: "Save address" }));

    expect(screen.getByLabelText("Country code (2-letter)")).toHaveAccessibleDescription(
      "Country must be a 2-letter code (e.g. US, DE, RS).",
    );
    expect(addAddressAction).not.toHaveBeenCalled();
  });

  it("keeps every typed value and shows the backend's reason when the save fails", async () => {
    const user = userEvent.setup();
    renderWithIntl(<AddressForm submitLabel="Save address" />);
    await fillRequired(user, "rs");
    await user.type(screen.getByLabelText("Phone (optional)"), "+381 11 123");
    await user.click(screen.getByRole("button", { name: "Save address" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("Postal code does not match the city.");
    await waitFor(() => expect(screen.getByLabelText("City")).toHaveValue("Beograd"));
    expect(screen.getByLabelText("Street address")).toHaveValue("Knez Mihailova 1");
    expect(screen.getByLabelText("Phone (optional)")).toHaveValue("+381 11 123");
  });
});
