/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };

import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type ServerFeaturesQueryVariables = Exact<{ [key: string]: never; }>;


export type ServerFeaturesQuery = { serverFeatures: { b2b: boolean, signals: boolean, ai: boolean } };


export const ServerFeaturesDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ServerFeatures"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"serverFeatures"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"b2b"}},{"kind":"Field","name":{"kind":"Name","value":"signals"}},{"kind":"Field","name":{"kind":"Name","value":"ai"}}]}}]}}]} as unknown as DocumentNode<ServerFeaturesQuery, ServerFeaturesQueryVariables>;