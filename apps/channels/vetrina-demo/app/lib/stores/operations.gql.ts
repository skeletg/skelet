/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
import type * as Types from '../../graphql/schema.types';

import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type ProductKind =
  | 'BUNDLE'
  | 'CONFIGURABLE'
  | 'GIFTCARD'
  | 'GROUPED'
  | 'KIT'
  | 'SIMPLE'
  | 'SUBSCRIPTION';

export type PublicStoresInput = {
  city?: string | null | undefined;
  inStockOnly?: boolean | null | undefined;
  latitude?: number | null | undefined;
  limit?: number | null | undefined;
  longitude?: number | null | undefined;
  offset?: number | null | undefined;
  pickupOnly?: boolean | null | undefined;
  preferredStoreId?: string | number | null | undefined;
  productId?: string | number | null | undefined;
  search?: string | null | undefined;
};

export type StoreStockStatus =
  | 'IN_STOCK'
  | 'LOW_STOCK'
  | 'OUT_OF_STOCK'
  | 'UNKNOWN';

export type PublicStoresQueryVariables = Exact<{
  input?: Types.PublicStoresInput | null | undefined;
}>;


export type PublicStoresQuery = { publicStores: { enabled: boolean, stockCheckingAvailable: boolean, hasPickupStores: boolean, productEligible: boolean, totalCount: number, cities: Array<string>, items: Array<{ id: string, slug: string, name: string, line1: string, line2: string, city: string, region: string, postalCode: string, countryCode: string, latitude: number | null, longitude: number | null, timezone: string, hours: unknown, phone: string, pickupEnabled: boolean, distanceKm: number | null, stockStatus: Types.StoreStockStatus | null }>, preferredStore: { id: string, slug: string, name: string, line1: string, line2: string, city: string, region: string, postalCode: string, countryCode: string, latitude: number | null, longitude: number | null, timezone: string, hours: unknown, phone: string, pickupEnabled: boolean, distanceKm: number | null, stockStatus: Types.StoreStockStatus | null } | null } };

export type PublicStoreFieldsFragment = { id: string, slug: string, name: string, line1: string, line2: string, city: string, region: string, postalCode: string, countryCode: string, latitude: number | null, longitude: number | null, timezone: string, hours: unknown, phone: string, pickupEnabled: boolean, distanceKm: number | null, stockStatus: Types.StoreStockStatus | null };

export type StoreProductQueryVariables = Exact<{
  id: string | number;
}>;


export type StoreProductQuery = { product: { id: string, name: string | null, sku: string, kind: Types.ProductKind, children: Array<{ id: string, name: string | null, sku: string, kind: Types.ProductKind }> } };

export const PublicStoreFieldsFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"PublicStoreFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PublicStore"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"line1"}},{"kind":"Field","name":{"kind":"Name","value":"line2"}},{"kind":"Field","name":{"kind":"Name","value":"city"}},{"kind":"Field","name":{"kind":"Name","value":"region"}},{"kind":"Field","name":{"kind":"Name","value":"postalCode"}},{"kind":"Field","name":{"kind":"Name","value":"countryCode"}},{"kind":"Field","name":{"kind":"Name","value":"latitude"}},{"kind":"Field","name":{"kind":"Name","value":"longitude"}},{"kind":"Field","name":{"kind":"Name","value":"timezone"}},{"kind":"Field","name":{"kind":"Name","value":"hours"}},{"kind":"Field","name":{"kind":"Name","value":"phone"}},{"kind":"Field","name":{"kind":"Name","value":"pickupEnabled"}},{"kind":"Field","name":{"kind":"Name","value":"distanceKm"}},{"kind":"Field","name":{"kind":"Name","value":"stockStatus"}}]}}]} as unknown as DocumentNode<PublicStoreFieldsFragment, unknown>;
export const PublicStoresDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"PublicStores"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"PublicStoresInput"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"publicStores"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"enabled"}},{"kind":"Field","name":{"kind":"Name","value":"stockCheckingAvailable"}},{"kind":"Field","name":{"kind":"Name","value":"hasPickupStores"}},{"kind":"Field","name":{"kind":"Name","value":"productEligible"}},{"kind":"Field","name":{"kind":"Name","value":"totalCount"}},{"kind":"Field","name":{"kind":"Name","value":"cities"}},{"kind":"Field","name":{"kind":"Name","value":"items"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"PublicStoreFields"}}]}},{"kind":"Field","name":{"kind":"Name","value":"preferredStore"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"PublicStoreFields"}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"PublicStoreFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PublicStore"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"line1"}},{"kind":"Field","name":{"kind":"Name","value":"line2"}},{"kind":"Field","name":{"kind":"Name","value":"city"}},{"kind":"Field","name":{"kind":"Name","value":"region"}},{"kind":"Field","name":{"kind":"Name","value":"postalCode"}},{"kind":"Field","name":{"kind":"Name","value":"countryCode"}},{"kind":"Field","name":{"kind":"Name","value":"latitude"}},{"kind":"Field","name":{"kind":"Name","value":"longitude"}},{"kind":"Field","name":{"kind":"Name","value":"timezone"}},{"kind":"Field","name":{"kind":"Name","value":"hours"}},{"kind":"Field","name":{"kind":"Name","value":"phone"}},{"kind":"Field","name":{"kind":"Name","value":"pickupEnabled"}},{"kind":"Field","name":{"kind":"Name","value":"distanceKm"}},{"kind":"Field","name":{"kind":"Name","value":"stockStatus"}}]}}]} as unknown as DocumentNode<PublicStoresQuery, PublicStoresQueryVariables>;
export const StoreProductDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"StoreProduct"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"product"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"sku"}},{"kind":"Field","name":{"kind":"Name","value":"kind"}},{"kind":"Field","name":{"kind":"Name","value":"children"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"sku"}},{"kind":"Field","name":{"kind":"Name","value":"kind"}}]}}]}}]}}]} as unknown as DocumentNode<StoreProductQuery, StoreProductQueryVariables>;