/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
import type * as Types from '../../graphql/signals.types';

import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type CreateCustomerSegmentInput = {
  description?: string | null | undefined;
  isActive?: boolean | null | undefined;
  name: string;
  slug: string;
};

export type UpdateCustomerSegmentInput = {
  description?: string | null | undefined;
  id: string | number;
  isActive?: boolean | null | undefined;
  name?: string | null | undefined;
  slug?: string | null | undefined;
};

export type CustomerSegmentsListQueryVariables = Exact<{
  limit?: number | null | undefined;
  offset?: number | null | undefined;
}>;


export type CustomerSegmentsListQuery = { customerSegments: { totalCount: number, items: Array<{ id: string, slug: string, name: string, description: string, isActive: boolean, memberCount: number }> } };

export type CreateCustomerSegmentMutationVariables = Exact<{
  input: Types.CreateCustomerSegmentInput;
}>;


export type CreateCustomerSegmentMutation = { createCustomerSegment: { id: string, slug: string, name: string } };

export type UpdateCustomerSegmentMutationVariables = Exact<{
  input: Types.UpdateCustomerSegmentInput;
}>;


export type UpdateCustomerSegmentMutation = { updateCustomerSegment: { id: string, slug: string, name: string, isActive: boolean } };

export type DeleteCustomerSegmentMutationVariables = Exact<{
  id: string | number;
}>;


export type DeleteCustomerSegmentMutation = { deleteCustomerSegment: boolean };


export const CustomerSegmentsListDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"CustomerSegmentsList"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"offset"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"customerSegments"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}},{"kind":"Argument","name":{"kind":"Name","value":"offset"},"value":{"kind":"Variable","name":{"kind":"Name","value":"offset"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"items"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"memberCount"}}]}},{"kind":"Field","name":{"kind":"Name","value":"totalCount"}}]}}]}}]} as unknown as DocumentNode<CustomerSegmentsListQuery, CustomerSegmentsListQueryVariables>;
export const CreateCustomerSegmentDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateCustomerSegment"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"CreateCustomerSegmentInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createCustomerSegment"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}}]}}]} as unknown as DocumentNode<CreateCustomerSegmentMutation, CreateCustomerSegmentMutationVariables>;
export const UpdateCustomerSegmentDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateCustomerSegment"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateCustomerSegmentInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateCustomerSegment"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}}]}}]}}]} as unknown as DocumentNode<UpdateCustomerSegmentMutation, UpdateCustomerSegmentMutationVariables>;
export const DeleteCustomerSegmentDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteCustomerSegment"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteCustomerSegment"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}]}]}}]} as unknown as DocumentNode<DeleteCustomerSegmentMutation, DeleteCustomerSegmentMutationVariables>;