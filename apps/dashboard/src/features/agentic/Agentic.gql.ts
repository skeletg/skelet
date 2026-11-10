/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
import type * as Types from '../../graphql/schema.types';

import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type AgenticState =
  | 'ATTENTION'
  | 'OFF'
  | 'ON';

export type ConfigScopeInput = {
  channelId?: string | number | null | undefined;
  kind: ConfigScopeKind;
  locale?: string | null | undefined;
};

export type ConfigScopeKind =
  | 'CHANNEL'
  | 'DEFAULT';

export type AgenticReportQueryVariables = Exact<{
  channelId: string | number;
  days: number;
}>;


export type AgenticReportQuery = { assistantReport: { days: number, turns: number, failedTurns: number, unansweredTurns: number, cartTurns: number, checkoutTurns: number, searchOverviews: number, usage: { inputTokens: number, outputTokens: number, cacheReadTokens: number, cacheWriteTokens: number, estimatedCostUsd: number | null }, orders: Array<{ currencyCode: string, orderCount: number, assistantOrders: number, assistantRevenue: number, agentOrders: number, agentRevenue: number }>, daily: Array<{ day: string, turns: number, failedTurns: number, unansweredTurns: number, cartTurns: number, checkoutTurns: number, assistantOrders: number, agentOrders: number }> } };

export type AgenticStatusQueryVariables = Exact<{
  channelId: string | number;
}>;


export type AgenticStatusQuery = { agenticStatus: Array<{ key: string, label: string, state: Types.AgenticState, detail: string, settingsPath: string }> };

export type AssistantNameQueryVariables = Exact<{
  scope?: Types.ConfigScopeInput | null | undefined;
}>;


export type AssistantNameQuery = { configValue: { path: string, value: string } };


export const AgenticReportDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"AgenticReport"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"days"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"assistantReport"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"channelId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}}},{"kind":"Argument","name":{"kind":"Name","value":"days"},"value":{"kind":"Variable","name":{"kind":"Name","value":"days"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"days"}},{"kind":"Field","name":{"kind":"Name","value":"turns"}},{"kind":"Field","name":{"kind":"Name","value":"failedTurns"}},{"kind":"Field","name":{"kind":"Name","value":"unansweredTurns"}},{"kind":"Field","name":{"kind":"Name","value":"cartTurns"}},{"kind":"Field","name":{"kind":"Name","value":"checkoutTurns"}},{"kind":"Field","name":{"kind":"Name","value":"searchOverviews"}},{"kind":"Field","name":{"kind":"Name","value":"usage"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"inputTokens"}},{"kind":"Field","name":{"kind":"Name","value":"outputTokens"}},{"kind":"Field","name":{"kind":"Name","value":"cacheReadTokens"}},{"kind":"Field","name":{"kind":"Name","value":"cacheWriteTokens"}},{"kind":"Field","name":{"kind":"Name","value":"estimatedCostUsd"}}]}},{"kind":"Field","name":{"kind":"Name","value":"orders"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"currencyCode"}},{"kind":"Field","name":{"kind":"Name","value":"orderCount"}},{"kind":"Field","name":{"kind":"Name","value":"assistantOrders"}},{"kind":"Field","name":{"kind":"Name","value":"assistantRevenue"}},{"kind":"Field","name":{"kind":"Name","value":"agentOrders"}},{"kind":"Field","name":{"kind":"Name","value":"agentRevenue"}}]}},{"kind":"Field","name":{"kind":"Name","value":"daily"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"day"}},{"kind":"Field","name":{"kind":"Name","value":"turns"}},{"kind":"Field","name":{"kind":"Name","value":"failedTurns"}},{"kind":"Field","name":{"kind":"Name","value":"unansweredTurns"}},{"kind":"Field","name":{"kind":"Name","value":"cartTurns"}},{"kind":"Field","name":{"kind":"Name","value":"checkoutTurns"}},{"kind":"Field","name":{"kind":"Name","value":"assistantOrders"}},{"kind":"Field","name":{"kind":"Name","value":"agentOrders"}}]}}]}}]}}]} as unknown as DocumentNode<AgenticReportQuery, AgenticReportQueryVariables>;
export const AgenticStatusDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"AgenticStatus"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"agenticStatus"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"channelId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"key"}},{"kind":"Field","name":{"kind":"Name","value":"label"}},{"kind":"Field","name":{"kind":"Name","value":"state"}},{"kind":"Field","name":{"kind":"Name","value":"detail"}},{"kind":"Field","name":{"kind":"Name","value":"settingsPath"}}]}}]}}]} as unknown as DocumentNode<AgenticStatusQuery, AgenticStatusQueryVariables>;
export const AssistantNameDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"AssistantName"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"scope"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"ConfigScopeInput"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"configValue"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"path"},"value":{"kind":"StringValue","value":"sales/assistant/name","block":false}},{"kind":"Argument","name":{"kind":"Name","value":"scope"},"value":{"kind":"Variable","name":{"kind":"Name","value":"scope"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"path"}},{"kind":"Field","name":{"kind":"Name","value":"value"}}]}}]}}]} as unknown as DocumentNode<AssistantNameQuery, AssistantNameQueryVariables>;