/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
import type * as Types from '../../graphql/schema.types';

import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type SaveFlowInput = {
  channelId: string | number;
  graph: unknown;
  name: string;
  triggerConfig?: unknown;
  triggerType: string;
};

export type FlowFieldsFragment = { id: string, channelId: string, name: string, triggerType: string, triggerConfig: unknown, graph: unknown, status: string, createdAt: string, updatedAt: string, deletedAt: string | null };

export type FlowsQueryVariables = Exact<{
  channelId: string | number;
  includeDeleted?: boolean | null | undefined;
}>;


export type FlowsQuery = { flows: Array<{ id: string, channelId: string, name: string, triggerType: string, triggerConfig: unknown, graph: unknown, status: string, createdAt: string, updatedAt: string, deletedAt: string | null }> };

export type FlowQueryVariables = Exact<{
  id: string | number;
}>;


export type FlowQuery = { flow: { id: string, channelId: string, name: string, triggerType: string, triggerConfig: unknown, graph: unknown, status: string, createdAt: string, updatedAt: string, deletedAt: string | null } | null };

export type FlowTriggerTypesQueryVariables = Exact<{ [key: string]: never; }>;


export type FlowTriggerTypesQuery = { flowTriggerTypes: Array<string> };

export type FlowConditionKindsQueryVariables = Exact<{ [key: string]: never; }>;


export type FlowConditionKindsQuery = { flowConditionKinds: Array<string> };

export type FlowMergeTagsQueryVariables = Exact<{ [key: string]: never; }>;


export type FlowMergeTagsQuery = { flowMergeTags: Array<string> };

export type FlowSegmentsQueryVariables = Exact<{ [key: string]: never; }>;


export type FlowSegmentsQuery = { customerSegments: { items: Array<{ id: string, name: string }> } };

export type CreateFlowMutationVariables = Exact<{
  input: Types.SaveFlowInput;
}>;


export type CreateFlowMutation = { createFlow: { id: string, channelId: string, name: string, triggerType: string, triggerConfig: unknown, graph: unknown, status: string, createdAt: string, updatedAt: string, deletedAt: string | null } };

export type UpdateFlowMutationVariables = Exact<{
  id: string | number;
  input: Types.SaveFlowInput;
}>;


export type UpdateFlowMutation = { updateFlow: { id: string, channelId: string, name: string, triggerType: string, triggerConfig: unknown, graph: unknown, status: string, createdAt: string, updatedAt: string, deletedAt: string | null } };

export type DeleteFlowMutationVariables = Exact<{
  id: string | number;
}>;


export type DeleteFlowMutation = { deleteFlow: boolean };

export type SetFlowStatusMutationVariables = Exact<{
  id: string | number;
  status: string;
}>;


export type SetFlowStatusMutation = { setFlowStatus: { id: string, channelId: string, name: string, triggerType: string, triggerConfig: unknown, graph: unknown, status: string, createdAt: string, updatedAt: string, deletedAt: string | null } };

export type RestoreFlowMutationVariables = Exact<{
  id: string | number;
}>;


export type RestoreFlowMutation = { restoreFlow: boolean };

export const FlowFieldsFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"FlowFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Flow"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"channelId"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"triggerType"}},{"kind":"Field","name":{"kind":"Name","value":"triggerConfig"}},{"kind":"Field","name":{"kind":"Name","value":"graph"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"deletedAt"}}]}}]} as unknown as DocumentNode<FlowFieldsFragment, unknown>;
export const FlowsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"Flows"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"includeDeleted"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Boolean"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"flows"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"channelId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}}},{"kind":"Argument","name":{"kind":"Name","value":"includeDeleted"},"value":{"kind":"Variable","name":{"kind":"Name","value":"includeDeleted"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"FlowFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"FlowFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Flow"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"channelId"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"triggerType"}},{"kind":"Field","name":{"kind":"Name","value":"triggerConfig"}},{"kind":"Field","name":{"kind":"Name","value":"graph"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"deletedAt"}}]}}]} as unknown as DocumentNode<FlowsQuery, FlowsQueryVariables>;
export const FlowDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"Flow"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"flow"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"FlowFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"FlowFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Flow"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"channelId"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"triggerType"}},{"kind":"Field","name":{"kind":"Name","value":"triggerConfig"}},{"kind":"Field","name":{"kind":"Name","value":"graph"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"deletedAt"}}]}}]} as unknown as DocumentNode<FlowQuery, FlowQueryVariables>;
export const FlowTriggerTypesDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"FlowTriggerTypes"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"flowTriggerTypes"}}]}}]} as unknown as DocumentNode<FlowTriggerTypesQuery, FlowTriggerTypesQueryVariables>;
export const FlowConditionKindsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"FlowConditionKinds"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"flowConditionKinds"}}]}}]} as unknown as DocumentNode<FlowConditionKindsQuery, FlowConditionKindsQueryVariables>;
export const FlowMergeTagsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"FlowMergeTags"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"flowMergeTags"}}]}}]} as unknown as DocumentNode<FlowMergeTagsQuery, FlowMergeTagsQueryVariables>;
export const FlowSegmentsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"FlowSegments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"customerSegments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"items"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}}]}}]}}]} as unknown as DocumentNode<FlowSegmentsQuery, FlowSegmentsQueryVariables>;
export const CreateFlowDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateFlow"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"SaveFlowInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createFlow"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"FlowFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"FlowFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Flow"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"channelId"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"triggerType"}},{"kind":"Field","name":{"kind":"Name","value":"triggerConfig"}},{"kind":"Field","name":{"kind":"Name","value":"graph"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"deletedAt"}}]}}]} as unknown as DocumentNode<CreateFlowMutation, CreateFlowMutationVariables>;
export const UpdateFlowDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateFlow"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"SaveFlowInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateFlow"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}},{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"FlowFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"FlowFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Flow"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"channelId"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"triggerType"}},{"kind":"Field","name":{"kind":"Name","value":"triggerConfig"}},{"kind":"Field","name":{"kind":"Name","value":"graph"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"deletedAt"}}]}}]} as unknown as DocumentNode<UpdateFlowMutation, UpdateFlowMutationVariables>;
export const DeleteFlowDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteFlow"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteFlow"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}]}]}}]} as unknown as DocumentNode<DeleteFlowMutation, DeleteFlowMutationVariables>;
export const SetFlowStatusDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SetFlowStatus"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"status"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"setFlowStatus"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}},{"kind":"Argument","name":{"kind":"Name","value":"status"},"value":{"kind":"Variable","name":{"kind":"Name","value":"status"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"FlowFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"FlowFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Flow"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"channelId"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"triggerType"}},{"kind":"Field","name":{"kind":"Name","value":"triggerConfig"}},{"kind":"Field","name":{"kind":"Name","value":"graph"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"deletedAt"}}]}}]} as unknown as DocumentNode<SetFlowStatusMutation, SetFlowStatusMutationVariables>;
export const RestoreFlowDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RestoreFlow"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"restoreFlow"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}]}]}}]} as unknown as DocumentNode<RestoreFlowMutation, RestoreFlowMutationVariables>;