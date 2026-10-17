import "server-only";
import { gqlFetch } from "@/app/lib/graphql";
import { PublicStoresDocument, StoreProductDocument } from "./operations.gql";
import type { PublicStoresInput } from "@/app/graphql/schema.types";

export async function readPublicStores(input: PublicStoresInput = {}, locale?: string) {
  const result = await gqlFetch(PublicStoresDocument, { input }, { locale });
  return result.data.publicStores;
}
export async function readStoreProduct(id: string, locale?: string) {
  const result = await gqlFetch(StoreProductDocument, { id }, { locale });
  return result.data.product;
}
