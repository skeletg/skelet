/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
import type * as Types from '../../graphql/schema.types';

import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type ChannelLocalesQueryVariables = Exact<{ [key: string]: never; }>;


export type ChannelLocalesQuery = { currentChannel: { defaultLocale: string, locales: Array<{ code: string, hreflangValue: string }> } | null };


export const ChannelLocalesDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ChannelLocales"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"currentChannel"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"defaultLocale"}},{"kind":"Field","name":{"kind":"Name","value":"locales"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"hreflangValue"}}]}}]}}]}}]} as unknown as DocumentNode<ChannelLocalesQuery, ChannelLocalesQueryVariables>;