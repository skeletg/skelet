/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
import type * as Types from '../../graphql/schema.types';

import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type PresentmentCurrenciesQueryVariables = Exact<{ [key: string]: never; }>;


export type PresentmentCurrenciesQuery = { presentmentCurrencies: Array<{ code: string, name: string, symbol: string, decimals: number, symbolPosition: string, factorFromBase: number, rateUpdatedAt: string }> };


export const PresentmentCurrenciesDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"PresentmentCurrencies"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"presentmentCurrencies"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"symbol"}},{"kind":"Field","name":{"kind":"Name","value":"decimals"}},{"kind":"Field","name":{"kind":"Name","value":"symbolPosition"}},{"kind":"Field","name":{"kind":"Name","value":"factorFromBase"}},{"kind":"Field","name":{"kind":"Name","value":"rateUpdatedAt"}}]}}]}}]} as unknown as DocumentNode<PresentmentCurrenciesQuery, PresentmentCurrenciesQueryVariables>;