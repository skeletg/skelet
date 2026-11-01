import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";

import { parseForm, type FormResult } from "@skelet/storefront-core/forms/schema";
import { trackOrderSchema } from "@skelet/storefront-core/order/schemas";
import { renderWithIntl } from "@/app/test/render";

// Same schema as the real action, answering the way it does when nothing matched
const trackOrderAction = vi.fn(async (_prev: FormResult | undefined, formData: FormData): Promise<FormResult> => {
  const submission = parseForm(formData, trackOrderSchema);
  if (submission.status !== "success") return submission.reply();
  return submission.reply({ formErrors: ["orderNotFound"] });
});
vi.mock("@skelet/storefront-next/order/actions", () => ({
  trackOrderAction: (prev: FormResult | undefined, fd: FormData) => trackOrderAction(prev, fd),
}));

const { TrackOrderForm } = await import("./TrackOrderForm");

beforeEach(() => {
  trackOrderAction.mockClear();
});

it("prefills the order number and email it was linked with", () => {
  renderWithIntl(<TrackOrderForm number="ORD-000123" email="a@example.com" />);
  expect(screen.getByLabelText("Order number")).toHaveValue("ORD-000123");
  expect(screen.getByLabelText("Email")).toHaveValue("a@example.com");
  expect(screen.getByLabelText("Postcode")).toHaveValue("");
});

it("stops a blank submit on the client", async () => {
  const user = userEvent.setup();
  renderWithIntl(<TrackOrderForm />);
  await user.click(screen.getByRole("button", { name: "Find my order" }));
  expect(await screen.findAllByText("This field is required.")).toHaveLength(3);
  expect(trackOrderAction).not.toHaveBeenCalled();
});

it("shows the not-found message and keeps what was typed", async () => {
  const user = userEvent.setup();
  renderWithIntl(<TrackOrderForm />);
  await user.type(screen.getByLabelText("Order number"), "ORD-999999");
  await user.type(screen.getByLabelText("Email"), "a@example.com");
  await user.type(screen.getByLabelText("Postcode"), "11000");
  await user.click(screen.getByRole("button", { name: "Find my order" }));
  expect(await screen.findByText(/We couldn't find an order with those details/)).toBeInTheDocument();
  expect(screen.getByLabelText("Order number")).toHaveValue("ORD-999999");
});
