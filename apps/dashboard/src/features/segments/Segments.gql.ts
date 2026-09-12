/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
import type * as Types from '../../graphql/schema.types';

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

export type SegmentsPageQueryVariables = Exact<{
  limit?: number | null | undefined;
  offset?: number | null | undefined;
}>;


export type SegmentsPageQuery = { customerSegments: { totalCount: number, items: Array<{ id: string, slug: string, name: string, description: string, isActive: boolean, memberCount: number, rules: unknown, updatedAt: string }> } };

export type SegmentDetailQueryVariables = Exact<{
  id: string | number;
}>;


export type SegmentDetailQuery = { customerSegment: { id: string, slug: string, name: string, description: string, isActive: boolean, memberCount: number, rules: unknown, createdAt: string, updatedAt: string, members: Array<{ id: string, joinedAt: string }> } | null };

export type CreateSegmentMutationVariables = Exact<{
  input: Types.CreateCustomerSegmentInput;
}>;


export type CreateSegmentMutation = { createCustomerSegment: { id: string } };

export type UpdateSegmentMutationVariables = Exact<{
  input: Types.UpdateCustomerSegmentInput;
}>;


export type UpdateSegmentMutation = { updateCustomerSegment: { id: string } };

export type DeleteSegmentMutationVariables = Exact<{
  id: string | number;
}>;


export type DeleteSegmentMutation = { deleteCustomerSegment: boolean };

export type SetSegmentRulesMutationVariables = Exact<{
  segmentId: string | number;
  rules?: unknown;
}>;


export type SetSegmentRulesMutation = { setSegmentRules: { id: string } };

export type RefreshSegmentMembershipMutationVariables = Exact<{
  segmentId: string | number;
}>;


export type RefreshSegmentMembershipMutation = { refreshSegment: number };

export type AssignCustomersToSegmentMutationVariables = Exact<{
  segmentId: string | number;
  customerIds: Array<string | number> | string | number;
}>;


export type AssignCustomersToSegmentMutation = { assignCustomersToSegment: boolean };

export type RemoveCustomerFromSegmentMutationVariables = Exact<{
  segmentId: string | number;
  customerId: string | number;
}>;


export type RemoveCustomerFromSegmentMutation = { removeCustomerFromSegment: boolean };

export type ClearSegmentMembershipMutationVariables = Exact<{
  segmentId: string | number;
}>;


export type ClearSegmentMembershipMutation = { clearSegmentMembership: boolean };


export const SegmentsPageDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"SegmentsPage"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"offset"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"customerSegments"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}},{"kind":"Argument","name":{"kind":"Name","value":"offset"},"value":{"kind":"Variable","name":{"kind":"Name","value":"offset"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"items"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"memberCount"}},{"kind":"Field","name":{"kind":"Name","value":"rules"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"totalCount"}}]}}]}}]} as unknown as DocumentNode<SegmentsPageQuery, SegmentsPageQueryVariables>;
export const SegmentDetailDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"SegmentDetail"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"customerSegment"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"memberCount"}},{"kind":"Field","name":{"kind":"Name","value":"rules"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"members"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"IntValue","value":"100"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"joinedAt"}}]}}]}}]}}]} as unknown as DocumentNode<SegmentDetailQuery, SegmentDetailQueryVariables>;
export const CreateSegmentDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateSegment"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"CreateCustomerSegmentInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createCustomerSegment"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]} as unknown as DocumentNode<CreateSegmentMutation, CreateSegmentMutationVariables>;
export const UpdateSegmentDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateSegment"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateCustomerSegmentInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateCustomerSegment"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]} as unknown as DocumentNode<UpdateSegmentMutation, UpdateSegmentMutationVariables>;
export const DeleteSegmentDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteSegment"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteCustomerSegment"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}]}]}}]} as unknown as DocumentNode<DeleteSegmentMutation, DeleteSegmentMutationVariables>;
export const SetSegmentRulesDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SetSegmentRules"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"segmentId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"rules"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"JSON"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"setSegmentRules"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"segmentId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"segmentId"}}},{"kind":"Argument","name":{"kind":"Name","value":"rules"},"value":{"kind":"Variable","name":{"kind":"Name","value":"rules"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]} as unknown as DocumentNode<SetSegmentRulesMutation, SetSegmentRulesMutationVariables>;
export const RefreshSegmentMembershipDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RefreshSegmentMembership"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"segmentId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"refreshSegment"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"segmentId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"segmentId"}}}]}]}}]} as unknown as DocumentNode<RefreshSegmentMembershipMutation, RefreshSegmentMembershipMutationVariables>;
export const AssignCustomersToSegmentDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"AssignCustomersToSegment"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"segmentId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"customerIds"}},"type":{"kind":"NonNullType","type":{"kind":"ListType","type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"assignCustomersToSegment"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"segmentId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"segmentId"}}},{"kind":"Argument","name":{"kind":"Name","value":"customerIds"},"value":{"kind":"Variable","name":{"kind":"Name","value":"customerIds"}}}]}]}}]} as unknown as DocumentNode<AssignCustomersToSegmentMutation, AssignCustomersToSegmentMutationVariables>;
export const RemoveCustomerFromSegmentDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RemoveCustomerFromSegment"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"segmentId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"customerId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"removeCustomerFromSegment"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"segmentId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"segmentId"}}},{"kind":"Argument","name":{"kind":"Name","value":"customerId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"customerId"}}}]}]}}]} as unknown as DocumentNode<RemoveCustomerFromSegmentMutation, RemoveCustomerFromSegmentMutationVariables>;
export const ClearSegmentMembershipDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"ClearSegmentMembership"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"segmentId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"clearSegmentMembership"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"segmentId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"segmentId"}}}]}]}}]} as unknown as DocumentNode<ClearSegmentMembershipMutation, ClearSegmentMembershipMutationVariables>;