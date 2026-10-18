/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };

import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type SchemaFormAttributesQueryVariables = Exact<{ [key: string]: never; }>;


export type SchemaFormAttributesQuery = { attributes: Array<{ id: string, slug: string, name: string, values: Array<{ slug: string, name: string }> }> };

export type SchemaFormCustomerGroupsQueryVariables = Exact<{ [key: string]: never; }>;


export type SchemaFormCustomerGroupsQuery = { customerGroups: Array<{ id: string, slug: string, name: string }> };

export type SchemaFormSkuPickerProductsQueryVariables = Exact<{
  search?: string | null | undefined;
}>;


export type SchemaFormSkuPickerProductsQuery = { productsPaged: { items: Array<{ id: string, sku: string, name: string | null }> } };


export const SchemaFormAttributesDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"SchemaFormAttributes"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"attributes"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"values"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}}]}}]}}]} as unknown as DocumentNode<SchemaFormAttributesQuery, SchemaFormAttributesQueryVariables>;
export const SchemaFormCustomerGroupsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"SchemaFormCustomerGroups"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"customerGroups"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}}]}}]} as unknown as DocumentNode<SchemaFormCustomerGroupsQuery, SchemaFormCustomerGroupsQueryVariables>;
export const SchemaFormSkuPickerProductsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"SchemaFormSkuPickerProducts"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"search"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"productsPaged"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"search"},"value":{"kind":"Variable","name":{"kind":"Name","value":"search"}}},{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"IntValue","value":"20"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"items"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"sku"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}}]}}]}}]} as unknown as DocumentNode<SchemaFormSkuPickerProductsQuery, SchemaFormSkuPickerProductsQueryVariables>;