import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { loginSchema } from "@/app/lib/auth/schemas";
import { parseForm, type FormResult } from "@/app/lib/forms/schema";
import { localeLinkMock } from "@/app/test/locale-link-mock";
import { renderWithIntl } from "@/app/test/render";

vi.mock("@/app/components/LocaleLink", () => localeLinkMock);

// The real action talks to the backend; this one runs the same schema and answers the way it does on a rejected
// sign-in, so the form sees exactly the SubmissionResult shape it gets in production
const loginAction = vi.fn(
  async (_prev: FormResult | undefined, formData: FormData): Promise<FormResult> => {
    const submission = parseForm(formData, loginSchema);
    if (submission.status !== "success") return submission.reply({ hideFields: ["password"] });
    return submission.reply({
      formErrors: ["Invalid email or password."],
      hideFields: ["password"],
    });
  },
);
vi.mock("@/app/lib/auth/actions", () => ({
  loginAction: (prev: FormResult | undefined, fd: FormData) => loginAction(prev, fd),
}));

const { LoginForm } = await import("@/app/components/auth/LoginForm");

describe("LoginForm", () => {
  beforeEach(() => {
    loginAction.mockClear();
  });

  it("stops a blank submit on the client with an error on each field", async () => {
    const user = userEvent.setup();
    renderWithIntl(<LoginForm />);
    await user.click(screen.getByRole("button", { name: "Sign in" }));

    expect(await screen.findAllByText("This field is required.")).toHaveLength(2);
    expect(screen.getByLabelText("Email")).toHaveAttribute("aria-invalid", "true");
    expect(loginAction).not.toHaveBeenCalled();
  });

  it("flags a malformed email when the field loses focus", async () => {
    const user = userEvent.setup();
    renderWithIntl(<LoginForm />);
    await user.type(screen.getByLabelText("Email"), "not-an-email");
    await user.tab();

    expect(await screen.findByText("Enter a valid email address.")).toBeInTheDocument();
  });

  it("keeps the email, clears the password and shows the backend's error after a rejected sign-in", async () => {
    const user = userEvent.setup();
    renderWithIntl(<LoginForm returnTo="/cart" />);
    await user.type(screen.getByLabelText("Email"), "ana@example.com");
    await user.type(screen.getByLabelText("Password"), "wrong-password");
    await user.click(screen.getByRole("button", { name: "Sign in" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("Invalid email or password.");
    await waitFor(() => expect(screen.getByLabelText("Email")).toHaveValue("ana@example.com"));
    expect(screen.getByLabelText("Password")).toHaveValue("");

    const sent = loginAction.mock.calls[0][1];
    expect(sent.get("returnTo")).toBe("/cart");
  });
});
