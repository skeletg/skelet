import { fireEvent, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { parseForm } from "@/app/lib/forms/schema";
import { productPickSchema, productSearchSchema } from "@/app/lib/stores/schemas";
import type { StoreProduct } from "@/app/lib/stores/types";
import type { ProductPickResult, ProductSearchResult } from "@/app/lib/stores/actions";
import { renderWithIntl } from "@/app/test/render";

const simpleId = "1f8b64d2-96f1-4fbd-8252-f42bd396f9c1";
const parentId = "2f8b64d2-96f1-4fbd-8252-f42bd396f9c2";
const childId = "3f8b64d2-96f1-4fbd-8252-f42bd396f9c3";
const products: Record<string, StoreProduct | null> = {
  [simpleId]: { id: simpleId, name: "Mug", sku: "MUG", kind: "SIMPLE", children: [] },
  [parentId]: { id: parentId, name: "Hoodie", sku: "HOODIE", kind: "CONFIGURABLE", children: [{ id: childId, name: "", sku: "HOODIE-S", kind: "SIMPLE" }] },
};

// Same schemas and SubmissionResult shape as the real actions, minus the backend
let searchFails = false;
const searchProducts = vi.fn(async (_prev: ProductSearchResult | undefined, formData: FormData): Promise<ProductSearchResult> => {
  const submission = parseForm(formData, productSearchSchema);
  if (submission.status !== "success") return submission.reply();
  if (searchFails) return submission.reply({ formErrors: ["unavailable"] });
  const hits = Object.values(products).filter((p): p is StoreProduct => !!p && (p.name ?? "").toLowerCase().includes(submission.value.q.toLowerCase()));
  return { ...submission.reply(), hits: hits.map(({ id, name, sku }) => ({ id, name, sku })) };
});
const pickProduct = vi.fn(async (_prev: ProductPickResult | undefined, formData: FormData): Promise<ProductPickResult> => {
  const submission = parseForm(formData, productPickSchema);
  return submission.status === "success" ? { ...submission.reply(), product: products[submission.value.productId] ?? null } : submission.reply();
});
vi.mock("@/app/lib/stores/actions", () => ({
  searchStoreProductsAction: (prev: ProductSearchResult | undefined, fd: FormData) => searchProducts(prev, fd),
  pickStoreProductAction: (prev: ProductPickResult | undefined, fd: FormData) => pickProduct(prev, fd),
}));

const { ProductPicker } = await import("./ProductPicker");

const onPick = vi.fn();
const onClear = vi.fn();
const search = (text: string) => {
  fireEvent.change(screen.getByLabelText("Product name or SKU"), { target: { value: text } });
  fireEvent.click(screen.getByRole("button", { name: "Search" }));
};
beforeEach(() => { searchFails = false; onPick.mockClear(); onClear.mockClear(); searchProducts.mockClear(); pickProduct.mockClear(); });

describe("ProductPicker", () => {
  it("stops a blank search on the client", async () => {
    renderWithIntl(<ProductPicker onPick={onPick} onClear={onClear} />);
    fireEvent.click(screen.getByRole("button", { name: "Search" }));
    expect(await screen.findByText("This field is required.")).toBeInTheDocument();
    expect(searchProducts).not.toHaveBeenCalled();
  });
  it("says so when nothing matches", async () => {
    renderWithIntl(<ProductPicker onPick={onPick} onClear={onClear} />);
    search("sofa");
    expect(await screen.findByText("No products match your search.")).toBeInTheDocument();
  });
  it("picks a simple product outright", async () => {
    renderWithIntl(<ProductPicker onPick={onPick} onClear={onClear} />);
    search("mug");
    fireEvent.click(await screen.findByRole("button", { name: "Mug · MUG" }));
    await waitFor(() => expect(onPick).toHaveBeenLastCalledWith({ id: simpleId, name: "Mug" }));
    expect(screen.queryByLabelText("Variant")).not.toBeInTheDocument();
  });
  it("waits for a variant of a configurable product, falling back to its SKU for a name", async () => {
    renderWithIntl(<ProductPicker onPick={onPick} onClear={onClear} />);
    search("hoodie");
    fireEvent.click(await screen.findByRole("button", { name: "Hoodie · HOODIE" }));
    await waitFor(() => expect(onPick).toHaveBeenLastCalledWith(null));
    fireEvent.change(await screen.findByLabelText("Variant"), { target: { value: childId } });
    expect(onPick).toHaveBeenLastCalledWith({ id: childId, name: "HOODIE-S" });
    fireEvent.click(screen.getByRole("button", { name: "Clear product" }));
    expect(onClear).toHaveBeenCalled();
  });
  it("reports a failed search", async () => {
    searchFails = true;
    renderWithIntl(<ProductPicker onPick={onPick} onClear={onClear} />);
    search("mug");
    expect(await screen.findByRole("alert")).toHaveTextContent("We could not load products. Please search again.");
  });
});
