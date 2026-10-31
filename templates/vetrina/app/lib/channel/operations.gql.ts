/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
import type * as Types from '../../graphql/schema.types';

import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type CurrentChannelQueryVariables = Exact<{ [key: string]: never; }>;


export type CurrentChannelQuery = { currentChannel: { id: string, slug: string, name: string, defaultLocale: string, currencyCode: string, publicUrl: string, currency: { code: string, symbol: string, symbolPosition: string, decimals: number } } | null };


export const CurrentChannelDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"CurrentChannel"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"currentChannel"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"defaultLocale"}},{"kind":"Field","name":{"kind":"Name","value":"currencyCode"}},{"kind":"Field","name":{"kind":"Name","value":"currency"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"symbol"}},{"kind":"Field","name":{"kind":"Name","value":"symbolPosition"}},{"kind":"Field","name":{"kind":"Name","value":"decimals"}}]}},{"kind":"Field","name":{"kind":"Name","value":"publicUrl"}}]}}]}}]} as unknown as DocumentNode<CurrentChannelQuery, CurrentChannelQueryVariables>;