import { fireEvent, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { signupSchema } from "@skelet/storefront-core/beacons/schemas";
import type { SignupResult } from "@skelet/storefront-next/beacons/actions";
import { parseForm, replyReset } from "@skelet/storefront-core/forms/schema";
import { renderWithIntl } from "@/app/test/render";

// Same schema and SubmissionResult shape as the real action, minus the backend
let reply: "ok" | "sentinel" = "ok";
const submit = vi.fn(async (_prev: SignupResult | undefined, formData: FormData): Promise<SignupResult> => {
  const submission = parseForm(formData, signupSchema);
  if (submission.status !== "success") return submission.reply();
  return reply === "ok" ? { ...replyReset(submission), message: "Welcome aboard" } : submission.reply({ formErrors: ["signup form not found"] });
});
vi.mock("@skelet/storefront-next/beacons/actions", () => ({
  submitSignupAction: (prev: SignupResult | undefined, fd: FormData) => submit(prev, fd),
}));

const { SignupForms } = await import("./SignupForms");

const inline = { id: "f1", slug: "newsletter", type: "inline", heading: "News", body: "", buttonLabel: "Join", successMessage: "Thanks", collectName: true, display: { trigger: "immediate", delaySeconds: 0, scrollPercent: 0, frequencyDays: 30 } };
const fetchMock = vi.fn<typeof fetch>(async () => new Response(JSON.stringify([inline])));
beforeEach(() => {
  reply = "ok";
  submit.mockClear();
  fetchMock.mockClear();
  vi.stubGlobal("fetch", fetchMock);
  document.cookie = "signup_form_newsletter=; max-age=0; path=/";
});
afterEach(() => vi.unstubAllGlobals());

describe("SignupForms", () => {
  it("loads the channel's active forms from the beacon feed", async () => {
    renderWithIntl(<SignupForms channelId="channel-1" />);
    await screen.findByText("News");
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/t/forms");
    expect(new Headers(init.headers).get("X-Channel-Id")).toBe("channel-1");
  });
  it("requires an email before submitting", async () => {
    renderWithIntl(<SignupForms channelId="channel-1" />);
    fireEvent.click(await screen.findByRole("button", { name: "Join" }));
    expect(await screen.findByText("This field is required.")).toBeInTheDocument();
    expect(submit).not.toHaveBeenCalled();
  });
  it("subscribes, shows the backend's message and won't show the form again", async () => {
    renderWithIntl(<SignupForms channelId="channel-1" />);
    fireEvent.change(await screen.findByLabelText("First name"), { target: { value: "Ana" } });
    fireEvent.change(screen.getByLabelText("Email"), { target: { value: "ana@example.com" } });
    fireEvent.click(screen.getByRole("button", { name: "Join" }));
    expect(await screen.findByRole("status")).toHaveTextContent("Welcome aboard");
    expect(Object.fromEntries(submit.mock.calls[0][1])).toEqual({ form: "newsletter", name: "Ana", email: "ana@example.com" });
    expect(document.cookie).toContain("signup_form_newsletter=1");
  });
  it("shows the backend's own wording for a rejected submission", async () => {
    reply = "sentinel";
    renderWithIntl(<SignupForms channelId="channel-1" />);
    fireEvent.change(await screen.findByLabelText("Email"), { target: { value: "ana@example.com" } });
    fireEvent.click(screen.getByRole("button", { name: "Join" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("signup form not found");
  });
});
