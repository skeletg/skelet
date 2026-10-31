/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
import type * as Types from '../../graphql/schema.types';

import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type CartOfferState =
  | 'COMPLETED'
  | 'DRAFT'
  | 'EXPIRED'
  | 'OPENED'
  | 'REVOKED'
  | 'SENT';

export type AccountCountsQueryVariables = Exact<{ [key: string]: never; }>;


export type AccountCountsQuery = { wishlistProductIds: Array<string>, myCartOffers: Array<{ id: string, offer: { state: Types.CartOfferState } | null }> };


export const AccountCountsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"AccountCounts"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"wishlistProductIds"}},{"kind":"Field","name":{"kind":"Name","value":"myCartOffers"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"offer"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"state"}}]}}]}}]}}]} as unknown as DocumentNode<AccountCountsQuery, AccountCountsQueryVariables>;