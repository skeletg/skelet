import { fireEvent, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { backInStockSchema } from "@/app/lib/beacons/schemas";
import { parseForm, replyReset, type FormResult } from "@/app/lib/forms/schema";
import { renderWithIntl } from "@/app/test/render";

let identity: { email: string } | null = null;
vi.mock("@/app/lib/customer/client", () => ({ useCustomerIdentity: () => ({ identity, resolved: true }) }));

// Same schema and SubmissionResult shape as the real action, minus the backend
let fails = false;
const notify = vi.fn(async (_prev: FormResult | undefined, formData: FormData): Promise<FormResult> => {
  const submission = parseForm(formData, backInStockSchema);
  if (submission.status !== "success") return submission.reply();
  return fails ? submission.reply({ formErrors: ["notifyFailed"] }) : replyReset(submission);
});
vi.mock("@/app/lib/beacons/actions", () => ({
  notifyBackInStockAction: (prev: FormResult | undefined, fd: FormData) => notify(prev, fd),
}));

const { BackInStockForm } = await import("./BackInStockForm");

const productId = "1f8b64d2-96f1-4fbd-8252-f42bd396f9c1";
const emailBox = () => screen.getByLabelText("Out of stock - get notified when it returns");
beforeEach(() => {
  identity = null;
  fails = false;
  notify.mockClear();
});

describe("BackInStockForm", () => {
  it("stops a blank or malformed email on the client", async () => {
    renderWithIntl(<BackInStockForm productId={productId} />);
    fireEvent.click(screen.getByRole("button", { name: "Notify me" }));
    expect(await screen.findByText("This field is required.")).toBeInTheDocument();
    fireEvent.change(emailBox(), { target: { value: "nope" } });
    fireEvent.click(screen.getByRole("button", { name: "Notify me" }));
    await waitFor(() => expect(emailBox()).toHaveAttribute("aria-invalid", "true"));
    expect(notify).not.toHaveBeenCalled();
  });
  it("subscribes the shopper to this product and confirms", async () => {
    renderWithIntl(<BackInStockForm productId={productId} />);
    fireEvent.change(emailBox(), { target: { value: "a@example.com" } });
    fireEvent.click(screen.getByRole("button", { name: "Notify me" }));
    expect(await screen.findByRole("status")).toHaveTextContent("You're on the list");
    expect(Object.fromEntries(notify.mock.calls[0][1])).toEqual({ productId, email: "a@example.com" });
  });
  it("prefills a signed-in shopper's email", () => {
    identity = { email: "me@example.com" };
    renderWithIntl(<BackInStockForm productId={productId} />);
    expect(emailBox()).toHaveValue("me@example.com");
  });
  it("fills a late identity's email without overwriting one the shopper typed", async () => {
    const view = renderWithIntl(<BackInStockForm productId={productId} />);
    identity = { email: "me@example.com" };
    view.rerender(<BackInStockForm productId={productId} />);
    await waitFor(() => expect(emailBox()).toHaveValue("me@example.com"));
    view.unmount();
    identity = null;
    const typed = renderWithIntl(<BackInStockForm productId={productId} />);
    fireEvent.change(emailBox(), { target: { value: "own@example.com" } });
    identity = { email: "me@example.com" };
    typed.rerender(<BackInStockForm productId={productId} />);
    expect(emailBox()).toHaveValue("own@example.com");
  });
  it("keeps the email and explains when the request fails", async () => {
    fails = true;
    renderWithIntl(<BackInStockForm productId={productId} />);
    fireEvent.change(emailBox(), { target: { value: "a@example.com" } });
    fireEvent.click(screen.getByRole("button", { name: "Notify me" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("We could not add you to the list.");
    expect(emailBox()).toHaveValue("a@example.com");
  });
});
