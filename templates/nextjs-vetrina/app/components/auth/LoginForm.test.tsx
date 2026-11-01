import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { loginSchema } from "@skelet/storefront-core/auth/schemas";
import { parseForm, type FormResult } from "@skelet/storefront-core/forms/schema";
import { localeLinkMock } from "@/app/test/locale-link-mock";
import { renderWithIntl } from "@/app/test/render";

vi.mock("@/app/components/LocaleLink", () => localeLinkMock);

// The real action talks to the backend; this one runs the same schema and answers the way it does on a rejected
// sign-in, so the form sees exactly the SubmissionResult shape it gets in production
const loginAction = vi.fn(
  async (_prev: FormResult | undefined, formData: FormData): Promise<FormResult> => {
    const submission = parseForm(formData, loginSchema);
    if (submission.status !== "success") return submission.reply({ hideFields: ["password"] });
    if (submission.value.email === "unverified@example.com") {
      return submission.reply({ formErrors: ["emailNotVerified"], hideFields: ["password"] });
    }
    return submission.reply({
      formErrors: ["Invalid email or password."],
      hideFields: ["password"],
    });
  },
);
const resendVerificationAction = vi.fn(async (fd: FormData) => void fd);
vi.mock("@skelet/storefront-next/auth/actions", () => ({
  loginAction: (prev: FormResult | undefined, fd: FormData) => loginAction(prev, fd),
  resendVerificationAction: (fd: FormData) => resendVerificationAction(fd),
}));

const { LoginForm } = await import("@/app/components/auth/LoginForm");

describe("LoginForm", () => {
  beforeEach(() => {
    loginAction.mockClear();
    resendVerificationAction.mockClear();
  });

  it("stops a blank submit on the client with an error on each field", async () => {
    const user = userEvent.setup();
    renderWithIntl(<LoginForm />);
    await user.click(screen.getByRole("button", { name: "Sign in" }));

    expect(await screen.findAllByText("This field is required.")).toHaveLength(2);
    expect(screen.getByLabelText("Email")).toHaveAttribute("aria-invalid", "true");
    expect(loginAction).not.toHaveBeenCalled();
  });

  it("waits for a submit before flagging a field, then clears the error as soon as it's fixed", async () => {
    const user = userEvent.setup();
    renderWithIntl(<LoginForm />);
    await user.type(screen.getByLabelText("Email"), "not-an-email");
    await user.tab();
    expect(screen.queryByText("Enter a valid email address.")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Sign in" }));
    expect(await screen.findByText("Enter a valid email address.")).toBeInTheDocument();

    await user.clear(screen.getByLabelText("Email"));
    await user.type(screen.getByLabelText("Email"), "ana@example.com");
    expect(screen.queryByText("Enter a valid email address.")).not.toBeInTheDocument();
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

  it("offers no resend link on an ordinary failed sign-in", async () => {
    const user = userEvent.setup();
    renderWithIntl(<LoginForm />);
    await user.type(screen.getByLabelText("Email"), "ana@example.com");
    await user.type(screen.getByLabelText("Password"), "wrong-password");
    await user.click(screen.getByRole("button", { name: "Sign in" }));

    await screen.findByRole("alert");
    expect(screen.queryByRole("button", { name: "Send a new confirmation link" })).not.toBeInTheDocument();
  });

  it("offers a fresh confirmation link for the typed address when the account isn't verified yet", async () => {
    const user = userEvent.setup();
    renderWithIntl(<LoginForm returnTo="/cart" />);
    await user.type(screen.getByLabelText("Email"), "unverified@example.com");
    await user.type(screen.getByLabelText("Password"), "password123");
    await user.click(screen.getByRole("button", { name: "Sign in" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("Confirm your email address before signing in.");
    await user.click(screen.getByRole("button", { name: "Send a new confirmation link" }));

    await waitFor(() => expect(resendVerificationAction).toHaveBeenCalledTimes(1));
    const sent = resendVerificationAction.mock.calls[0][0];
    expect(sent.get("email")).toBe("unverified@example.com");
    expect(sent.get("returnTo")).toBe("/cart");
  });
});
