import { fireEvent, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { WebFormSubmitResult } from "@skelet/storefront-next/webforms/actions";
import { renderWithIntl } from "@/app/test/render";
import type { InlineForm } from "./webform";

// The backend validates CMS-defined fields, so the mock answers the way it does: field errors, a general one, or ok
let answer: WebFormSubmitResult = { ok: true, successMessage: "Got it", errors: [] };
const submit = vi.fn<(input: unknown) => Promise<WebFormSubmitResult>>(async () => answer);
vi.mock("@skelet/storefront-next/webforms/actions", () => ({
  submitWebFormAction: (input: unknown) => submit(input),
  presignWebFormUploadAction: vi.fn(),
}));

const { WebFormClient } = await import("./WebFormClient");

const form: InlineForm = {
  id: "form-1",
  name: "Contact",
  slug: "contact",
  settings: { submitLabel: "Send" },
  fields: [
    { key: "source", type: "hidden", label: "", defaultValue: "footer" },
    { key: "email", type: "email", label: "Your email", required: true },
    { key: "topic", type: "text", label: "Topic" },
  ],
};
const send = () => fireEvent.click(screen.getByRole("button", { name: "Send" }));
beforeEach(() => {
  answer = { ok: true, successMessage: "Got it", errors: [] };
  submit.mockClear();
});

describe("WebFormClient", () => {
  it("submits the visible answers with the hidden defaults, then shows the success message", async () => {
    renderWithIntl(<WebFormClient form={form} />);
    fireEvent.change(screen.getByLabelText(/Your email/), { target: { value: "a@example.com" } });
    send();
    expect(await screen.findByText("Got it")).toBeInTheDocument();
    expect(submit).toHaveBeenCalledWith(expect.objectContaining({
      formId: "form-1",
      values: { source: "footer", email: "a@example.com" },
      honeypot: "",
    }));
  });
  it("puts the backend's field errors under their fields and keeps the answers", async () => {
    answer = { ok: false, errors: [{ field: "email", message: "Enter a valid email." }] };
    renderWithIntl(<WebFormClient form={form} />);
    fireEvent.change(screen.getByLabelText(/Your email/), { target: { value: "nope" } });
    send();
    expect(await screen.findByText("Enter a valid email.")).toBeInTheDocument();
    expect(screen.getByText("Please fix the errors below.")).toBeInTheDocument();
    expect(screen.getByLabelText(/Your email/)).toHaveValue("nope");
  });
  it("shows a general error, and clears it on the next attempt", async () => {
    answer = { ok: false, errors: [{ field: "", message: "Too many submissions." }] };
    renderWithIntl(<WebFormClient form={form} />);
    send();
    expect(await screen.findByText("Too many submissions.")).toBeInTheDocument();
    answer = { ok: true, successMessage: "", errors: [] };
    send();
    expect(await screen.findByText("Thanks - your response has been received.")).toBeInTheDocument();
  });
  it("disables the button while the submission is in flight", async () => {
    let finish: (r: WebFormSubmitResult) => void = () => {};
    submit.mockImplementationOnce(() => new Promise((resolve) => { finish = resolve; }));
    renderWithIntl(<WebFormClient form={form} />);
    send();
    expect(await screen.findByRole("button", { name: "Submitting…" })).toBeDisabled();
    finish({ ok: true, successMessage: "Done", errors: [] });
    await waitFor(() => expect(screen.getByText("Done")).toBeInTheDocument());
  });
});
