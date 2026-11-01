import { fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { parseForm } from "@skelet/storefront-core/forms/schema";
import { productPickSchema, productSearchSchema, storeFiltersSchema } from "@skelet/storefront-core/stores/schemas";
import { preferenceKey, type StoreProduct, type StoresResult } from "@skelet/storefront-core/stores/types";
import type { ProductPickResult, ProductSearchResult, StoresFormResult } from "@skelet/storefront-next/stores/actions";
import { localeLinkMock } from "@/app/test/locale-link-mock";
import { renderWithIntl } from "@/app/test/render";

vi.mock("@/app/components/LocaleLink", () => localeLinkMock);

const channel = "test-channel";
const productA = "1f8b64d2-96f1-4fbd-8252-f42bd396f9c1";
const productB = "2f8b64d2-96f1-4fbd-8252-f42bd396f9c2";
const store = { id: "9f8b64d2-96f1-4fbd-8252-f42bd396f9c8", slug: "central", name: "Central", line1: "Main 1", line2: "", city: "Belgrade", region: "", postalCode: "11000", countryCode: "RS", latitude: 44.8, longitude: 20.4, timezone: "Europe/Belgrade", hours: { monday: { open: "09:00", close: "18:00" } }, phone: "123", pickupEnabled: false, distanceKm: null, stockStatus: "IN_STOCK" as const };
const initial: StoresResult = { enabled: true, hasPickupStores: false, stockCheckingAvailable: true, productEligible: true, cities: ["Belgrade", "Novi Sad"], preferredStore: null, items: [store], totalCount: 1 };

// The real actions talk to the backend; these run the same schemas and answer in the same SubmissionResult shape
let answer: (query: Record<string, unknown>) => StoresResult | null = () => initial;
const findStores = vi.fn(async (_prev: StoresFormResult | undefined, formData: FormData): Promise<StoresFormResult> => {
  const submission = parseForm(formData, storeFiltersSchema);
  if (submission.status !== "success") return submission.reply();
  const stores = answer(submission.value);
  return stores ? { ...submission.reply(), stores, query: submission.value } : submission.reply({ formErrors: ["unavailable"] });
});
const hoodie: StoreProduct = { id: productA, name: "Hoodie", sku: "HOODIE", kind: "CONFIGURABLE", children: [{ id: productB, name: "Small", sku: "S", kind: "SIMPLE" }] };
const searchProducts = vi.fn(async (_prev: ProductSearchResult | undefined, formData: FormData): Promise<ProductSearchResult> => {
  const submission = parseForm(formData, productSearchSchema);
  return submission.status === "success" ? { ...submission.reply(), hits: [{ id: productA, name: "Hoodie", sku: "HOODIE" }] } : submission.reply();
});
const pickProduct = vi.fn(async (_prev: ProductPickResult | undefined, formData: FormData): Promise<ProductPickResult> => {
  const submission = parseForm(formData, productPickSchema);
  return submission.status === "success" ? { ...submission.reply(), product: hoodie } : submission.reply();
});
vi.mock("@skelet/storefront-next/stores/actions", () => ({
  findStoresAction: (prev: StoresFormResult | undefined, fd: FormData) => findStores(prev, fd),
  searchStoreProductsAction: (prev: ProductSearchResult | undefined, fd: FormData) => searchProducts(prev, fd),
  pickStoreProductAction: (prev: ProductPickResult | undefined, fd: FormData) => pickProduct(prev, fd),
}));

const { StoreFinder } = await import("./StoreFinder");
const { StoresPage } = await import("./StoresPage");
const { StoreAvailability } = await import("./StoreAvailability");

const lastQuery = () => Object.fromEntries(findStores.mock.calls.at(-1)![1] as FormData);
beforeEach(() => { localStorage.clear(); answer = () => initial; findStores.mockClear(); searchProducts.mockClear(); pickProduct.mockClear(); });
afterEach(() => vi.unstubAllGlobals());

describe("store finder", () => {
  it("applies filters as they change and persists a channel-specific preferred store", async () => {
    answer = (q) => ({ ...initial, preferredStore: q.preferredStoreId ? store : null });
    renderWithIntl(<StoreFinder productId={productA} />);
    await screen.findByText("Central");
    fireEvent.change(screen.getByLabelText("City"), { target: { value: "Belgrade" } });
    await waitFor(() => expect(lastQuery()).toMatchObject({ city: "Belgrade", productId: productA }));
    fireEvent.click(await screen.findByLabelText("In stock only"));
    await waitFor(() => expect(lastQuery()).toMatchObject({ inStockOnly: "on" }));
    fireEvent.click(await screen.findByRole("button", { name: "Set as my store" }));
    expect(localStorage.getItem(preferenceKey(channel))).toBe(store.id);
    expect(localStorage.getItem(preferenceKey("other-channel"))).toBeNull();
    await waitFor(() => expect(lastQuery()).toMatchObject({ preferredStoreId: store.id }));
    expect(localStorage.getItem(preferenceKey(channel))).toBe(store.id);
  });
  it("searches stores on Enter, not on every keystroke", async () => {
    renderWithIntl(<StoreFinder />);
    await screen.findByText("Central");
    const calls = findStores.mock.calls.length;
    const input = screen.getByLabelText("Store name, address or postal code");
    fireEvent.change(input, { target: { value: "novi" } });
    fireEvent.change(input, { target: { value: "novi sad" } });
    expect(findStores.mock.calls.length).toBe(calls);
    fireEvent.submit(input.closest("form")!);
    await waitFor(() => expect(lastQuery()).toMatchObject({ search: "novi sad" }));
    expect(findStores.mock.calls.length).toBe(calls + 1);
  });
  it("re-checks a new product with the shopper's filters still applied", async () => {
    const view = renderWithIntl(<StoreFinder productId={productA} />);
    await screen.findByText("Central");
    fireEvent.click(screen.getByLabelText("In stock only"));
    await waitFor(() => expect(lastQuery()).toMatchObject({ inStockOnly: "on" }));
    view.rerender(<StoreFinder productId={productB} />);
    await waitFor(() => expect(lastQuery()).toMatchObject({ productId: productB, inStockOnly: "on" }));
  });
  it("shows walk-in stores without pickup and offers a separate pickup filter", async () => {
    answer = () => ({ ...initial, hasPickupStores: true });
    renderWithIntl(<StoreFinder productId={productA} />);
    expect(await screen.findByText("In-store shopping")).toBeInTheDocument();
    fireEvent.click(screen.getByLabelText("Pickup available"));
    await waitFor(() => expect(lastQuery()).toMatchObject({ pickupOnly: "on" }));
  });
  it("does not turn missing availability into out of stock", async () => {
    answer = () => ({ ...initial, items: [{ ...store, stockStatus: "UNKNOWN" }] });
    renderWithIntl(<StoreFinder productId={productA} />);
    await screen.findByText("Availability unknown");
    expect(screen.queryByText("Out of stock")).not.toBeInTheDocument();
  });
  it("requests location only on click and keeps filters usable after denial", async () => {
    const getCurrentPosition = vi.fn((_success, failure) => failure());
    Object.defineProperty(navigator, "geolocation", { configurable: true, value: { getCurrentPosition } });
    renderWithIntl(<StoreFinder />);
    await screen.findByText("Central");
    expect(getCurrentPosition).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Near me" }));
    expect(await screen.findByText(/Your location is unavailable/)).toBeInTheDocument();
    expect(screen.getByLabelText("City")).toBeEnabled();
    expect(screen.getByLabelText("In stock only")).toBeDisabled();
  });
  it("sorts nearby without persisting coordinates", async () => {
    Object.defineProperty(navigator, "geolocation", { configurable: true, value: { getCurrentPosition: (success: (value: unknown) => void) => success({ coords: { latitude: 44.8, longitude: 20.4 } }) } });
    renderWithIntl(<StoreFinder />);
    await screen.findByText("Central");
    fireEvent.click(screen.getByRole("button", { name: "Near me" }));
    await waitFor(() => expect(lastQuery()).toMatchObject({ latitude: "44.8", longitude: "20.4" }));
    expect(localStorage.length).toBe(0);
  });
  it("pages with the pager's own offset and retries errors", async () => {
    answer = () => ({ ...initial, totalCount: 45 });
    renderWithIntl(<StoreFinder />);
    fireEvent.click(await screen.findByRole("button", { name: "Next" }));
    await waitFor(() => expect(lastQuery()).toMatchObject({ offset: "20" }));
    answer = () => null;
    const search = screen.getByLabelText("Store name, address or postal code");
    fireEvent.change(search, { target: { value: "missing" } });
    fireEvent.submit(search.closest("form")!);
    await screen.findByRole("button", { name: "Retry" });
    expect(screen.queryByText("Central")).not.toBeInTheDocument();
    answer = () => ({ ...initial, items: [], totalCount: 0 });
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByText("No stores match your filters.")).toBeInTheDocument();
    expect(lastQuery()).not.toHaveProperty("offset");
  });
});

describe("stores page", () => {
  it("keeps the directory available in flat mode", async () => {
    answer = () => ({ ...initial, stockCheckingAvailable: false, productEligible: false, items: [{ ...store, stockStatus: null }] });
    renderWithIntl(<StoresPage stockChecking={false} />);
    await screen.findByText("Central");
    expect(screen.queryByLabelText("Product name or SKU")).not.toBeInTheDocument();
    expect(screen.getByLabelText("In stock only")).toBeDisabled();
  });
  it("selects a product and then a specific variant", async () => {
    renderWithIntl(<StoresPage stockChecking />);
    await screen.findByText("Central");
    fireEvent.change(screen.getByLabelText("Product name or SKU"), { target: { value: "hoodie" } });
    fireEvent.click(screen.getByRole("button", { name: "Search" }));
    fireEvent.click(await screen.findByRole("button", { name: "Hoodie · HOODIE" }));
    fireEvent.change(await screen.findByLabelText("Variant"), { target: { value: productB } });
    await waitFor(() => expect(lastQuery()).toMatchObject({ productId: productB }));
  });
});

describe("product availability", () => {
  it("hides the entry point when disabled", async () => {
    answer = () => ({ ...initial, enabled: false });
    renderWithIntl(<StoreAvailability productId={productA} />);
    await waitFor(() => expect(findStores).toHaveBeenCalled());
    expect(screen.queryByText("Check availability in stores")).not.toBeInTheDocument();
  });
  it("requires variant selection and refreshes stock for a new variant", async () => {
    const view = renderWithIntl(<StoreAvailability needsVariant />);
    expect(await screen.findByRole("button", { name: "Check availability in stores" })).toBeDisabled();
    view.rerender(<StoreAvailability needsVariant productId={productB} />);
    await waitFor(() => expect(lastQuery()).toMatchObject({ productId: productB }));
    expect(await screen.findByRole("button", { name: "Check availability in stores" })).toBeEnabled();
  });
  it("shows the preferred store status and forgets an unavailable preferred store", async () => {
    localStorage.setItem(preferenceKey(channel), store.id);
    answer = () => ({ ...initial, preferredStore: store });
    const view = renderWithIntl(<StoreAvailability productId={productA} />);
    expect(await screen.findByText(/My store: Central/)).toBeInTheDocument();
    answer = () => initial;
    view.rerender(<StoreAvailability productId={productB} />);
    await waitFor(() => expect(localStorage.getItem(preferenceKey(channel))).toBeNull());
  });
});
