import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { profileSchema } from "@/app/lib/auth/schemas";
import { parseForm, type FormResult } from "@/app/lib/forms/schema";
import { renderWithIntl } from "@/app/test/render";

// Stands in for the server action, parsing with the same schema so a submission that gets here was valid
const updateProfileAction = vi.fn(async (_prev: FormResult | undefined, formData: FormData) => {
  const submission = parseForm(formData, profileSchema);
  return submission.status === "success" ? { ...submission.reply(), status: "success" as const } : submission.reply();
});
vi.mock("@/app/lib/auth/actions", () => ({
  updateProfileAction: (prev: FormResult | undefined, fd: FormData) => updateProfileAction(prev, fd),
}));

const { ProfileForm } = await import("@/app/[channel]/[locale]/account/profile/ProfileForm");

function renderForm(phoneCountry: string) {
  return renderWithIntl(
    <ProfileForm
      firstName="Ana"
      lastName="Ilić"
      phone=""
      // No store phone policy - a national number is read in the store's default country
      phoneSettings={{ restrictedToStoreCountries: false, lockedCountryCode: null, defaultCountryCode: phoneCountry, formats: [] }}
      email="ana@example.com"
    />,
  );
}

describe("ProfileForm phone", () => {
  it("reads a national number in the store's country and sends that country with it", async () => {
    updateProfileAction.mockClear();
    const user = userEvent.setup();
    renderForm("RS");
    await user.type(screen.getByLabelText("Phone"), "0641234567");
    expect(screen.getByLabelText("Phone")).toHaveValue("064 1234567");
    await user.click(screen.getByRole("button", { name: "Save changes" }));

    expect(updateProfileAction).toHaveBeenCalledTimes(1);
    const sent = updateProfileAction.mock.calls[0][1];
    expect(sent.get("phone")).toBe("064 1234567");
    expect(sent.get("phoneCountry")).toBe("RS");
  });

  it("refuses a number that can't be one in the store's country before posting", async () => {
    updateProfileAction.mockClear();
    const user = userEvent.setup();
    renderForm("US");
    await user.type(screen.getByLabelText("Phone"), "064 123 4567");
    await user.click(screen.getByRole("button", { name: "Save changes" }));

    expect(screen.getByLabelText("Phone")).toHaveAccessibleDescription(
      "Enter a valid phone number, or start it with + and the country code.",
    );
    expect(updateProfileAction).not.toHaveBeenCalled();
  });
});
