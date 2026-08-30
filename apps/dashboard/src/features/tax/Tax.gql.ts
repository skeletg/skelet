/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
import type * as Types from '../../graphql/schema.types';

import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type CreateTaxRuleInput = {
  channelId: string | number;
  conditions?: Array<TaxConditionInput> | null | undefined;
  effects?: Array<TaxEffectInput> | null | undefined;
  exclusivityGroup?: string | null | undefined;
  isActive?: boolean | null | undefined;
  name: string;
  priority?: number | null | undefined;
  sortOrder?: number | null | undefined;
};

export type TaxConditionInput = {
  params: unknown;
  paramsVersion?: number | null | undefined;
  sortOrder?: number | null | undefined;
  type: string;
};

export type TaxEffectInput = {
  params: unknown;
  paramsVersion?: number | null | undefined;
  sortOrder?: number | null | undefined;
  type: string;
};

export type UpdateTaxRuleInput = {
  conditions?: Array<TaxConditionInput> | null | undefined;
  effects?: Array<TaxEffectInput> | null | undefined;
  exclusivityGroup?: string | null | undefined;
  id: string | number;
  isActive?: boolean | null | undefined;
  name?: string | null | undefined;
  priority?: number | null | undefined;
  sortOrder?: number | null | undefined;
};

export type TaxRuleFieldsFragment = { id: string, channelId: string, name: string, priority: number, exclusivityGroup: string, sortOrder: number, isActive: boolean, createdAt: string, updatedAt: string, conditions: Array<{ type: string, params: unknown, paramsVersion: number, sortOrder: number }>, effects: Array<{ type: string, params: unknown, paramsVersion: number, sortOrder: number }> };

export type TaxRulesQueryVariables = Exact<{
  channelId?: string | number | null | undefined;
}>;


export type TaxRulesQuery = { taxRules: Array<{ id: string, channelId: string, name: string, priority: number, exclusivityGroup: string, sortOrder: number, isActive: boolean, createdAt: string, updatedAt: string, conditions: Array<{ type: string, params: unknown, paramsVersion: number, sortOrder: number }>, effects: Array<{ type: string, params: unknown, paramsVersion: number, sortOrder: number }> }> };

export type TaxRuleQueryVariables = Exact<{
  id: string | number;
}>;


export type TaxRuleQuery = { taxRule: { id: string, channelId: string, name: string, priority: number, exclusivityGroup: string, sortOrder: number, isActive: boolean, createdAt: string, updatedAt: string, conditions: Array<{ type: string, params: unknown, paramsVersion: number, sortOrder: number }>, effects: Array<{ type: string, params: unknown, paramsVersion: number, sortOrder: number }> } | null };

export type TaxRuleTypesQueryVariables = Exact<{ [key: string]: never; }>;


export type TaxRuleTypesQuery = { taxConditionTypes: Array<{ slug: string, label: string, description: string, paramsSchema: unknown }>, taxEffectTypes: Array<{ slug: string, label: string, description: string, paramsSchema: unknown }> };

export type CreateTaxRuleMutationVariables = Exact<{
  input: Types.CreateTaxRuleInput;
}>;


export type CreateTaxRuleMutation = { createTaxRule: { id: string, channelId: string, name: string, priority: number, exclusivityGroup: string, sortOrder: number, isActive: boolean, createdAt: string, updatedAt: string, conditions: Array<{ type: string, params: unknown, paramsVersion: number, sortOrder: number }>, effects: Array<{ type: string, params: unknown, paramsVersion: number, sortOrder: number }> } };

export type UpdateTaxRuleMutationVariables = Exact<{
  input: Types.UpdateTaxRuleInput;
}>;


export type UpdateTaxRuleMutation = { updateTaxRule: { id: string, channelId: string, name: string, priority: number, exclusivityGroup: string, sortOrder: number, isActive: boolean, createdAt: string, updatedAt: string, conditions: Array<{ type: string, params: unknown, paramsVersion: number, sortOrder: number }>, effects: Array<{ type: string, params: unknown, paramsVersion: number, sortOrder: number }> } };

export type DeleteTaxRuleMutationVariables = Exact<{
  id: string | number;
}>;


export type DeleteTaxRuleMutation = { deleteTaxRule: boolean };

export const TaxRuleFieldsFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"TaxRuleFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"TaxRule"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"channelId"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"priority"}},{"kind":"Field","name":{"kind":"Name","value":"exclusivityGroup"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"conditions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"params"}},{"kind":"Field","name":{"kind":"Name","value":"paramsVersion"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}}]}},{"kind":"Field","name":{"kind":"Name","value":"effects"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"params"}},{"kind":"Field","name":{"kind":"Name","value":"paramsVersion"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}}]}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]} as unknown as DocumentNode<TaxRuleFieldsFragment, unknown>;
export const TaxRulesDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"TaxRules"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"taxRules"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"channelId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"TaxRuleFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"TaxRuleFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"TaxRule"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"channelId"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"priority"}},{"kind":"Field","name":{"kind":"Name","value":"exclusivityGroup"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"conditions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"params"}},{"kind":"Field","name":{"kind":"Name","value":"paramsVersion"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}}]}},{"kind":"Field","name":{"kind":"Name","value":"effects"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"params"}},{"kind":"Field","name":{"kind":"Name","value":"paramsVersion"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}}]}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]} as unknown as DocumentNode<TaxRulesQuery, TaxRulesQueryVariables>;
export const TaxRuleDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"TaxRule"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"taxRule"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"TaxRuleFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"TaxRuleFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"TaxRule"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"channelId"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"priority"}},{"kind":"Field","name":{"kind":"Name","value":"exclusivityGroup"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"conditions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"params"}},{"kind":"Field","name":{"kind":"Name","value":"paramsVersion"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}}]}},{"kind":"Field","name":{"kind":"Name","value":"effects"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"params"}},{"kind":"Field","name":{"kind":"Name","value":"paramsVersion"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}}]}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]} as unknown as DocumentNode<TaxRuleQuery, TaxRuleQueryVariables>;
export const TaxRuleTypesDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"TaxRuleTypes"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"taxConditionTypes"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"label"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"paramsSchema"}}]}},{"kind":"Field","name":{"kind":"Name","value":"taxEffectTypes"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"label"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"paramsSchema"}}]}}]}}]} as unknown as DocumentNode<TaxRuleTypesQuery, TaxRuleTypesQueryVariables>;
export const CreateTaxRuleDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateTaxRule"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"CreateTaxRuleInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createTaxRule"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"TaxRuleFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"TaxRuleFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"TaxRule"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"channelId"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"priority"}},{"kind":"Field","name":{"kind":"Name","value":"exclusivityGroup"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"conditions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"params"}},{"kind":"Field","name":{"kind":"Name","value":"paramsVersion"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}}]}},{"kind":"Field","name":{"kind":"Name","value":"effects"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"params"}},{"kind":"Field","name":{"kind":"Name","value":"paramsVersion"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}}]}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]} as unknown as DocumentNode<CreateTaxRuleMutation, CreateTaxRuleMutationVariables>;
export const UpdateTaxRuleDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateTaxRule"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateTaxRuleInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateTaxRule"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"TaxRuleFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"TaxRuleFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"TaxRule"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"channelId"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"priority"}},{"kind":"Field","name":{"kind":"Name","value":"exclusivityGroup"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"conditions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"params"}},{"kind":"Field","name":{"kind":"Name","value":"paramsVersion"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}}]}},{"kind":"Field","name":{"kind":"Name","value":"effects"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"params"}},{"kind":"Field","name":{"kind":"Name","value":"paramsVersion"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}}]}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]} as unknown as DocumentNode<UpdateTaxRuleMutation, UpdateTaxRuleMutationVariables>;
export const DeleteTaxRuleDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteTaxRule"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteTaxRule"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}]}]}}]} as unknown as DocumentNode<DeleteTaxRuleMutation, DeleteTaxRuleMutationVariables>;