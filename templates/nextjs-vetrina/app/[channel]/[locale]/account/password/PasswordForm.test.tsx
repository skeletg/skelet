import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { changePasswordSchema } from "@skelet/storefront-core/auth/schemas";
import { parseForm, replyReset, type FormResult } from "@skelet/storefront-core/forms/schema";
import { renderWithIntl } from "@/app/test/render";

// Accepts any submission that passes the schema, the way the real action answers a successful change
vi.mock("@skelet/storefront-next/auth/actions", () => ({
  changePasswordAction: async (_prev: FormResult | undefined, formData: FormData) => {
    const submission = parseForm(formData, changePasswordSchema);
    if (submission.status !== "success") return submission.reply();
    return replyReset(submission);
  },
}));

const { PasswordForm } = await import("@/app/[channel]/[locale]/account/password/PasswordForm");

describe("PasswordForm", () => {
  it("puts a mismatch on the confirm field", async () => {
    const user = userEvent.setup();
    renderWithIntl(<PasswordForm />);
    await user.type(screen.getByLabelText("Current password"), "old-password");
    await user.type(screen.getByLabelText("New password"), "new-password-1");
    await user.type(screen.getByLabelText("Confirm new password"), "new-password-2");
    await user.click(screen.getByRole("button", { name: "Update password" }));

    expect(screen.getByLabelText("Confirm new password")).toHaveAccessibleDescription(
      "Passwords do not match.",
    );
  });

  it("confirms and empties every field after a successful change", async () => {
    const user = userEvent.setup();
    renderWithIntl(<PasswordForm />);
    await user.type(screen.getByLabelText("Current password"), "old-password");
    await user.type(screen.getByLabelText("New password"), "new-password-1");
    await user.type(screen.getByLabelText("Confirm new password"), "new-password-1");
    await user.click(screen.getByRole("button", { name: "Update password" }));

    expect(await screen.findByText("Password updated.")).toBeInTheDocument();
    await waitFor(() => expect(screen.getByLabelText("Current password")).toHaveValue(""));
    expect(screen.getByLabelText("New password")).toHaveValue("");
    expect(screen.getByLabelText("Confirm new password")).toHaveValue("");
  });
});
