/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };

import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type MasqueradeCustomerMutationVariables = Exact<{
  customerId: string | number;
}>;


export type MasqueradeCustomerMutation = { masqueradeCustomer: { storefrontPath: string, storefrontUrl: string, expiresInSeconds: number } };


export const MasqueradeCustomerDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MasqueradeCustomer"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"customerId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"masqueradeCustomer"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"customerId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"customerId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"storefrontPath"}},{"kind":"Field","name":{"kind":"Name","value":"storefrontUrl"}},{"kind":"Field","name":{"kind":"Name","value":"expiresInSeconds"}}]}}]}}]} as unknown as DocumentNode<MasqueradeCustomerMutation, MasqueradeCustomerMutationVariables>;