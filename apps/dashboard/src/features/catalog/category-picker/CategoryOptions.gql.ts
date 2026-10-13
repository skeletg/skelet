/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };

import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type CategoryOptionsQueryVariables = Exact<{ [key: string]: never; }>;


export type CategoryOptionsQuery = { categoryOptions: Array<{ id: string, parentId: string | null, slug: string, name: string | null, position: number, isPublished: boolean, productCount: number }> };


export const CategoryOptionsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"CategoryOptions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"categoryOptions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"parentId"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"position"}},{"kind":"Field","name":{"kind":"Name","value":"isPublished"}},{"kind":"Field","name":{"kind":"Name","value":"productCount"}}]}}]}}]} as unknown as DocumentNode<CategoryOptionsQuery, CategoryOptionsQueryVariables>;