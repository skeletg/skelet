import { getStoreAvailabilityEnabled } from "../config/read";
import { parseForm, type FormResult } from "../forms/schema";
import { gqlFetch } from "../graphql";
import { SearchProductsDocument } from "../search/operations.gql";
import { readPublicStores, readStoreProduct } from "./read";
import { productPickSchema, productSearchSchema, storeFiltersSchema, type StoreFilters } from "./schemas";
import { STORE_PAGE_SIZE, type ProductHit, type StoreProduct, type StoresResult } from "./types";

// query is the parsed filters the answer is for, so callers read the page and the preferred store it asked about typed
export type StoresFormResult = FormResult & { stores?: StoresResult; query?: StoreFilters };
export type ProductSearchResult = FormResult & { hits?: ProductHit[] };
export type ProductPickResult = FormResult & { product?: StoreProduct | null };

// The forms show their own "could not load" copy, so a failure carries no message of its own
const UNAVAILABLE = ["unavailable"];

export async function findStoresAction(
  _prev: StoresFormResult | undefined,
  formData: FormData,
): Promise<StoresFormResult> {
  const submission = parseForm(formData, storeFiltersSchema);
  if (submission.status !== "success") return submission.reply();
  try {
    const stores = await readPublicStores({ ...submission.value, limit: STORE_PAGE_SIZE });
    return { ...submission.reply(), stores, query: submission.value };
  } catch (err) {
    console.error("Store availability request failed", err);
    return submission.reply({ formErrors: UNAVAILABLE });
  }
}

export async function searchStoreProductsAction(
  _prev: ProductSearchResult | undefined,
  formData: FormData,
): Promise<ProductSearchResult> {
  const submission = parseForm(formData, productSearchSchema);
  if (submission.status !== "success") return submission.reply();
  try {
    if (!(await getStoreAvailabilityEnabled())) return submission.reply({ formErrors: UNAVAILABLE });
    const result = await gqlFetch(SearchProductsDocument, { query: submission.value.q, limit: 12 });
    return { ...submission.reply(), hits: result.data.searchProducts.items };
  } catch (err) {
    console.error("Store product search failed", err);
    return submission.reply({ formErrors: UNAVAILABLE });
  }
}

export async function pickStoreProductAction(
  _prev: ProductPickResult | undefined,
  formData: FormData,
): Promise<ProductPickResult> {
  const submission = parseForm(formData, productPickSchema);
  if (submission.status !== "success") return submission.reply();
  try {
    if (!(await getStoreAvailabilityEnabled())) return submission.reply({ formErrors: UNAVAILABLE });
    return { ...submission.reply(), product: await readStoreProduct(submission.value.productId) };
  } catch (err) {
    console.error("Store product lookup failed", err);
    return submission.reply({ formErrors: UNAVAILABLE });
  }
}
