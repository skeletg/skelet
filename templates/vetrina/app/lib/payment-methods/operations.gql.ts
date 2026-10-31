/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
import type * as Types from '../../graphql/schema.types';

import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type MyPaymentMethodsQueryVariables = Exact<{ [key: string]: never; }>;


export type MyPaymentMethodsQuery = { myPaymentMethods: Array<{ id: string, providerSlug: string, brand: string, last4: string, expMonth: number, expYear: number }> };

export type DeleteMyPaymentMethodMutationVariables = Exact<{
  id: string | number;
}>;


export type DeleteMyPaymentMethodMutation = { deleteMyPaymentMethod: boolean };

export type SavePaymentMethodMutationVariables = Exact<{
  providerSlug: string;
  token: string;
}>;


export type SavePaymentMethodMutation = { savePaymentMethod: { id: string, providerSlug: string, brand: string, last4: string, expMonth: number, expYear: number } };


export const MyPaymentMethodsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"MyPaymentMethods"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"myPaymentMethods"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"providerSlug"}},{"kind":"Field","name":{"kind":"Name","value":"brand"}},{"kind":"Field","name":{"kind":"Name","value":"last4"}},{"kind":"Field","name":{"kind":"Name","value":"expMonth"}},{"kind":"Field","name":{"kind":"Name","value":"expYear"}}]}}]}}]} as unknown as DocumentNode<MyPaymentMethodsQuery, MyPaymentMethodsQueryVariables>;
export const DeleteMyPaymentMethodDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteMyPaymentMethod"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteMyPaymentMethod"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}]}]}}]} as unknown as DocumentNode<DeleteMyPaymentMethodMutation, DeleteMyPaymentMethodMutationVariables>;
export const SavePaymentMethodDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SavePaymentMethod"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"providerSlug"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"token"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"savePaymentMethod"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"providerSlug"},"value":{"kind":"Variable","name":{"kind":"Name","value":"providerSlug"}}},{"kind":"Argument","name":{"kind":"Name","value":"token"},"value":{"kind":"Variable","name":{"kind":"Name","value":"token"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"providerSlug"}},{"kind":"Field","name":{"kind":"Name","value":"brand"}},{"kind":"Field","name":{"kind":"Name","value":"last4"}},{"kind":"Field","name":{"kind":"Name","value":"expMonth"}},{"kind":"Field","name":{"kind":"Name","value":"expYear"}}]}}]}}]} as unknown as DocumentNode<SavePaymentMethodMutation, SavePaymentMethodMutationVariables>;