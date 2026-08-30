/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };

import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type PulseEngagementQueryVariables = Exact<{
  channelId: string | number;
  sinceDays?: number | null | undefined;
}>;


export type PulseEngagementQuery = { pulseEngagement: { opensRaw: number, opensHuman: number, clicks: number, conversions: number, revenue: Array<{ currency: string, cents: number, orders: number }>, topLinks: Array<{ url: string, clicks: number }> } };


export const PulseEngagementDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"PulseEngagement"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"sinceDays"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"pulseEngagement"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"channelId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}}},{"kind":"Argument","name":{"kind":"Name","value":"sinceDays"},"value":{"kind":"Variable","name":{"kind":"Name","value":"sinceDays"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"opensRaw"}},{"kind":"Field","name":{"kind":"Name","value":"opensHuman"}},{"kind":"Field","name":{"kind":"Name","value":"clicks"}},{"kind":"Field","name":{"kind":"Name","value":"conversions"}},{"kind":"Field","name":{"kind":"Name","value":"revenue"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"currency"}},{"kind":"Field","name":{"kind":"Name","value":"cents"}},{"kind":"Field","name":{"kind":"Name","value":"orders"}}]}},{"kind":"Field","name":{"kind":"Name","value":"topLinks"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"clicks"}}]}}]}}]}}]} as unknown as DocumentNode<PulseEngagementQuery, PulseEngagementQueryVariables>;