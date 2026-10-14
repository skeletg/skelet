import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { NextIntlClientProvider } from "next-intl";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import messages from "@/messages/en.json";
import { StoreFinder } from "./StoreFinder";
import { StoreAvailability, StoresLink } from "./StoreAvailability";
import { preferenceKey, type StoresResult } from "@/app/lib/stores/types";

vi.mock("@/app/components/LocaleLink", () => ({ useChannelId: () => "channel-a", Link: ({ href, children }: { href: string; children: React.ReactNode }) => <a href={href}>{children}</a> }));
const store = { id: "9f8b64d2-96f1-4fbd-8252-f42bd396f9c8", slug: "central", name: "Central", line1: "Main 1", line2: "", city: "Belgrade", region: "", postalCode: "11000", countryCode: "RS", latitude: 44.8, longitude: 20.4, timezone: "Europe/Belgrade", hours: { monday: { open: "09:00", close: "18:00" } }, phone: "123", pickupEnabled: false, distanceKm: null, stockStatus: "IN_STOCK" as const };
const initial: StoresResult = { enabled: true, hasPickupStores: false, stockCheckingAvailable: true, productEligible: true, cities: ["Belgrade", "Novi Sad"], preferredStore: null, items: [store], totalCount: 1 };
const reply = (data: unknown, ok = true) => Promise.resolve({ ok, json: async () => data });
const fetchMock = vi.fn();
function mount(ui: React.ReactNode) { return render(<NextIntlClientProvider locale="en" messages={messages}>{ui}</NextIntlClientProvider>); }
beforeEach(() => { localStorage.clear(); fetchMock.mockReset().mockImplementation(() => reply(initial)); vi.stubGlobal("fetch", fetchMock); });
afterEach(() => vi.unstubAllGlobals());

describe("store finder", () => {
  it("filters stores, resets pagination and persists a channel-specific preferred store", async () => {
    fetchMock.mockImplementation((url: string) => reply({ ...initial, preferredStore: url.includes("preferredStoreId=") ? store : null }));
    mount(<StoreFinder productId="product-a" />);
    await screen.findByText("Central");
    await screen.findByRole("option", { name: "Belgrade" });
    fireEvent.change(screen.getByLabelText("City"), { target: { value: "Belgrade" } });
    await waitFor(() => expect(fetchMock.mock.calls.at(-1)?.[0]).toContain("city=Belgrade"));
    await screen.findByText("Central");
    fireEvent.click(screen.getByLabelText("In stock only"));
    await waitFor(() => expect(fetchMock.mock.calls.at(-1)?.[0]).toContain("inStockOnly=true"));
    await screen.findByText("Central");
    fireEvent.click(screen.getByRole("button", { name: "Set as my store" }));
    expect(localStorage.getItem(preferenceKey("channel-a"))).toBe(store.id);
    expect(localStorage.getItem(preferenceKey("channel-b"))).toBeNull();
    await waitFor(() => expect(fetchMock.mock.calls.at(-1)?.[0]).toContain(`preferredStoreId=${store.id}`));
  });
  it("shows walk-in stores without pickup and offers a separate pickup filter", async () => {
    fetchMock.mockImplementation(() => reply({ ...initial, hasPickupStores: true }));
    mount(<StoreFinder productId="product-a" />);
    expect(await screen.findByText("In-store shopping")).toBeInTheDocument();
    fireEvent.click(screen.getByLabelText("Pickup available"));
    await waitFor(() => expect(fetchMock.mock.calls.at(-1)?.[0]).toContain("pickupOnly=true"));
  });
  it("does not turn missing availability into out of stock", async () => {
    fetchMock.mockImplementation(() => reply({ ...initial, items: [{ ...store, stockStatus: "UNKNOWN" }] }));
    mount(<StoreFinder productId="product-a" />);
    await screen.findByText("Availability unknown");
    expect(screen.queryByText("Out of stock")).not.toBeInTheDocument();
  });
  it("requests location only on click and keeps filters usable after denial", async () => {
    const getCurrentPosition = vi.fn((_success, failure) => failure());
    Object.defineProperty(navigator, "geolocation", { configurable: true, value: { getCurrentPosition } });
    mount(<StoreFinder />);
    await screen.findByText("Central");
    expect(getCurrentPosition).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Near me" }));
    expect(await screen.findByText(/Your location is unavailable/)).toBeInTheDocument();
    expect(screen.getByLabelText("City")).toBeEnabled();
    expect(screen.getByLabelText("In stock only")).toBeDisabled();
  });
  it("sorts nearby without persisting coordinates", async () => {
    Object.defineProperty(navigator, "geolocation", { configurable: true, value: { getCurrentPosition: (success: (value: unknown) => void) => success({ coords: { latitude: 44.8, longitude: 20.4 } }) } });
    mount(<StoreFinder />);
    await screen.findByText("Central");
    fireEvent.click(screen.getByRole("button", { name: "Near me" }));
    await waitFor(() => expect(fetchMock.mock.calls.at(-1)?.[0]).toContain("latitude=44.8&longitude=20.4"));
    expect(localStorage.length).toBe(0);
  });
  it("clears stale rows during requests and retries errors", async () => {
    fetchMock.mockImplementationOnce(() => reply(initial)).mockImplementationOnce(() => reply({}, false)).mockImplementation(() => reply({ ...initial, items: [], totalCount: 0 }));
    mount(<StoreFinder />);
    await screen.findByText("Central");
    fireEvent.change(screen.getByLabelText("Store name, address or postal code"), { target: { value: "missing" } });
    expect(screen.queryByText("Central")).not.toBeInTheDocument();
    await screen.findByRole("button", { name: "Retry" });
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByText("No stores match your filters.")).toBeInTheDocument();
  });
  it("keeps the directory available in flat mode", async () => {
    fetchMock.mockImplementation(() => reply({ ...initial, stockCheckingAvailable: false, productEligible: false, items: [{ ...store, stockStatus: null }] }));
    mount(<StoreFinder allowProductSearch />);
    await screen.findByText("Central");
    expect(screen.queryByLabelText("Product name or SKU")).not.toBeInTheDocument();
    expect(screen.getByLabelText("In stock only")).toBeDisabled();
  });
  it("selects a product and then a specific variant", async () => {
    fetchMock.mockImplementation((url: string) => {
      if (url.includes("action=search")) return reply({ items: [{ id: "parent", name: "Hoodie", sku: "HOODIE" }] });
      if (url.includes("action=product")) return reply({ product: { id: "parent", name: "Hoodie", sku: "HOODIE", kind: "CONFIGURABLE", children: [{ id: "small", name: "Small", sku: "S", kind: "SIMPLE" }] } });
      return reply(initial);
    });
    mount(<StoreFinder allowProductSearch />);
    await screen.findByText("Central");
    fireEvent.change(screen.getByLabelText("Product name or SKU"), { target: { value: "hoodie" } });
    fireEvent.click(screen.getByRole("button", { name: "Search" }));
    fireEvent.click(await screen.findByRole("button", { name: "Hoodie · HOODIE" }));
    fireEvent.change(await screen.findByLabelText("Variant"), { target: { value: "small" } });
    await waitFor(() => expect(fetchMock.mock.calls.at(-1)?.[0]).toContain("productId=small"));
  });
});

describe("product availability and navigation", () => {
  it("hides entry points when disabled", async () => {
    fetchMock.mockImplementation(() => reply({ ...initial, enabled: false }));
    mount(<><StoresLink /><StoreAvailability productId="product-a" /></>);
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    expect(screen.queryByText("Stores")).not.toBeInTheDocument();
    expect(screen.queryByText("Check availability in stores")).not.toBeInTheDocument();
  });
  it("requires variant selection and refreshes stock for a new variant", async () => {
    const view = mount(<StoreAvailability needsVariant />);
    expect(await screen.findByRole("button", { name: "Check availability in stores" })).toBeDisabled();
    view.rerender(<NextIntlClientProvider locale="en" messages={messages}><StoreAvailability needsVariant productId="variant-b" /></NextIntlClientProvider>);
    await waitFor(() => expect(fetchMock.mock.calls.at(-1)?.[0]).toContain("productId=variant-b"));
    expect(await screen.findByRole("button", { name: "Check availability in stores" })).toBeEnabled();
  });
  it("shows the preferred store status and forgets an unavailable preferred store", async () => {
    localStorage.setItem(preferenceKey("channel-a"), store.id);
    fetchMock.mockImplementationOnce(() => reply({ ...initial, preferredStore: store })).mockImplementation(() => reply(initial));
    const view = mount(<StoreAvailability productId="product-a" />);
    expect(await screen.findByText(/My store: Central/)).toBeInTheDocument();
    view.rerender(<NextIntlClientProvider locale="en" messages={messages}><StoreAvailability productId="product-b" /></NextIntlClientProvider>);
    await waitFor(() => expect(localStorage.getItem(preferenceKey("channel-a"))).toBeNull());
  });
});
