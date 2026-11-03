import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";

import { CustomerProvider, useCustomerIdentity } from "./client";
import type { HeaderIdentity } from "./actions-client";

const mocks = vi.hoisted(() => ({ pathname: "/", load: vi.fn() }));
vi.mock("next/navigation", () => ({ usePathname: () => mocks.pathname }));
vi.mock("./actions-client", () => ({ loadHeaderIdentity: mocks.load }));

const customer = { id: "c1", firstName: "Ana", lastName: "Test", email: "ana@example.com" };
function Identity() {
  const { identity, resolved } = useCustomerIdentity();
  return <p>{resolved ? identity?.email ?? "guest" : "loading"}</p>;
}
const tree = () => <CustomerProvider><Identity /></CustomerProvider>;
beforeEach(() => { mocks.pathname = "/"; mocks.load.mockReset(); });

it("refreshes a preserved provider after login and logout navigation", async () => {
  mocks.load.mockResolvedValueOnce(null).mockResolvedValueOnce(customer).mockResolvedValueOnce(null);
  const view = render(tree());
  await screen.findByText("guest");
  mocks.pathname = "/account";
  view.rerender(tree());
  await screen.findByText(customer.email);
  mocks.pathname = "/account/login";
  view.rerender(tree());
  await screen.findByText("guest");
  expect(mocks.load).toHaveBeenCalledTimes(3);
});

it("keeps the signed-in identity on transient failure and retries on focus", async () => {
  mocks.load.mockResolvedValueOnce(customer).mockRejectedValueOnce(new Error("offline")).mockResolvedValueOnce(null);
  const view = render(tree());
  await screen.findByText(customer.email);
  mocks.pathname = "/products";
  view.rerender(tree());
  await waitFor(() => expect(mocks.load).toHaveBeenCalledTimes(2));
  expect(screen.getByText(customer.email)).toBeInTheDocument();
  fireEvent(window, new Event("focus"));
  await screen.findByText("guest");
});

it("does not show guest state when the first read fails", async () => {
  mocks.load.mockRejectedValueOnce(new Error("offline")).mockResolvedValueOnce(customer);
  render(tree());
  await waitFor(() => expect(mocks.load).toHaveBeenCalledTimes(1));
  expect(screen.getByText("loading")).toBeInTheDocument();
  fireEvent(document, new Event("visibilitychange"));
  await screen.findByText(customer.email);
});

it("ignores an older identity response after navigation", async () => {
  let finish!: (identity: HeaderIdentity) => void;
  mocks.load.mockImplementationOnce(() => new Promise<HeaderIdentity>((resolve) => { finish = resolve; })).mockResolvedValueOnce(customer);
  const view = render(tree());
  mocks.pathname = "/account";
  view.rerender(tree());
  await screen.findByText(customer.email);
  await act(async () => { finish(null); });
  expect(screen.getByText(customer.email)).toBeInTheDocument();
});

it("reads once for a return to the tab, and not again while the answer is fresh", async () => {
  vi.useFakeTimers({ toFake: ["Date"] });
  try {
    mocks.load.mockResolvedValue(customer);
    render(tree());
    await screen.findByText(customer.email);
    fireEvent(window, new Event("focus"));
    fireEvent(document, new Event("visibilitychange"));
    expect(mocks.load).toHaveBeenCalledTimes(1);
    vi.setSystemTime(Date.now() + 31_000);
    fireEvent(window, new Event("focus"));
    fireEvent(document, new Event("visibilitychange"));
    await waitFor(() => expect(mocks.load).toHaveBeenCalledTimes(2));
  } finally {
    vi.useRealTimers();
  }
});
