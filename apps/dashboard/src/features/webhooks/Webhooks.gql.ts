/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
import type * as Types from '../../graphql/schema.types';

import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type CreateWebhookInput = {
  channelId?: string | number | null | undefined;
  customHeaders?: unknown;
  eventTypes: Array<string>;
  failurePolicy?: WebhookFailurePolicy | null | undefined;
  kind: WebhookKind;
  name: string;
  targetUrl: string;
  timeoutMs?: number | null | undefined;
};

export type UpdateWebhookInput = {
  customHeaders?: unknown;
  eventTypes?: Array<string> | null | undefined;
  failurePolicy?: WebhookFailurePolicy | null | undefined;
  id: string | number;
  name?: string | null | undefined;
  targetUrl?: string | null | undefined;
  timeoutMs?: number | null | undefined;
};

export type WebhookCircuitState =
  | 'CLOSED'
  | 'HALF_OPEN'
  | 'OPEN';

export type WebhookFailurePolicy =
  | 'FAIL_CLOSED'
  | 'FAIL_OPEN';

export type WebhookKind =
  | 'ASYNC'
  | 'SYNC';

export type WebhookQueueStatus =
  | 'DEAD'
  | 'PENDING';

export type WebhooksQueryVariables = Exact<{ [key: string]: never; }>;


export type WebhooksQuery = { webhooks: Array<{ id: string, name: string, targetUrl: string, kind: Types.WebhookKind, isActive: boolean, eventTypes: Array<string>, failurePolicy: Types.WebhookFailurePolicy, createdAt: string, circuit: { state: Types.WebhookCircuitState } | null, channel: { id: string, slug: string, name: string } | null }> };

export type WebhookQueryVariables = Exact<{
  id: string | number;
}>;


export type WebhookQuery = { webhook: { id: string, name: string, targetUrl: string, kind: Types.WebhookKind, isActive: boolean, eventTypes: Array<string>, failurePolicy: Types.WebhookFailurePolicy, timeoutMs: number, customHeaders: unknown, payloadVersion: string, createdAt: string, updatedAt: string, circuit: { state: Types.WebhookCircuitState, consecutiveFailures: number, openedAt: string | null } | null, channel: { id: string, slug: string, name: string } | null } };

export type WebhookEventTypesQueryVariables = Exact<{ [key: string]: never; }>;


export type WebhookEventTypesQuery = { webhookEventTypes: Array<{ type: string, name: string, description: string, kind: Types.WebhookKind }> };

export type WebhookDeliveryStatsQueryVariables = Exact<{
  webhookId: string | number;
}>;


export type WebhookDeliveryStatsQuery = { webhookDeliveryStats: { total: number, succeeded: number, failed: number, successRate: number, avgSuccessMs: number, lastDeliveryAt: string | null, lastSuccessAt: string | null, lastFailureAt: string | null } };

export type WebhookDeliveriesQueryVariables = Exact<{
  webhookId: string | number;
  limit?: number | null | undefined;
}>;


export type WebhookDeliveriesQuery = { webhookDeliveries: Array<{ id: string, eventType: string, url: string, requestBody: string, requestHeaders: unknown, responseStatus: number | null, responseBody: string | null, responseHeaders: unknown, durationMs: number, success: boolean, errorMessage: string | null, attempt: number, createdAt: string }> };

export type CreateWebhookMutationVariables = Exact<{
  input: Types.CreateWebhookInput;
}>;


export type CreateWebhookMutation = { createWebhook: { rawSecret: string, webhook: { id: string, name: string, targetUrl: string, kind: Types.WebhookKind, isActive: boolean, eventTypes: Array<string>, createdAt: string, channel: { id: string, slug: string, name: string } | null } } };

export type UpdateWebhookMutationVariables = Exact<{
  input: Types.UpdateWebhookInput;
}>;


export type UpdateWebhookMutation = { updateWebhook: { id: string, name: string, targetUrl: string, eventTypes: Array<string>, failurePolicy: Types.WebhookFailurePolicy, timeoutMs: number, customHeaders: unknown, updatedAt: string, channel: { id: string, slug: string, name: string } | null } };

export type SetWebhookActiveMutationVariables = Exact<{
  id: string | number;
  active: boolean;
}>;


export type SetWebhookActiveMutation = { setWebhookActive: { id: string, isActive: boolean } };

export type RotateWebhookSecretMutationVariables = Exact<{
  id: string | number;
}>;


export type RotateWebhookSecretMutation = { rotateWebhookSecret: { rawSecret: string, webhook: { id: string } } };

export type DeleteWebhookMutationVariables = Exact<{
  id: string | number;
}>;


export type DeleteWebhookMutation = { deleteWebhook: boolean };

export type ClearWebhookDeliveriesMutationVariables = Exact<{
  webhookId: string | number;
}>;


export type ClearWebhookDeliveriesMutation = { clearWebhookDeliveries: number };

export type WebhookDeadLettersQueryVariables = Exact<{
  webhookId: string | number;
  limit?: number | null | undefined;
}>;


export type WebhookDeadLettersQuery = { webhookDeadLetters: Array<{ id: string, eventType: string, status: Types.WebhookQueueStatus, attempts: number, nextAttemptAt: string, lastError: string | null, payload: string, createdAt: string, updatedAt: string }> };

export type RetryWebhookDeadLetterMutationVariables = Exact<{
  id: string | number;
}>;


export type RetryWebhookDeadLetterMutation = { retryWebhookDeadLetter: { id: string, status: Types.WebhookQueueStatus, attempts: number, nextAttemptAt: string } };

export type ClearWebhookDeadLettersMutationVariables = Exact<{
  webhookId: string | number;
}>;


export type ClearWebhookDeadLettersMutation = { clearWebhookDeadLetters: number };

export type RedeliverWebhookDeliveryMutationVariables = Exact<{
  deliveryId: string | number;
}>;


export type RedeliverWebhookDeliveryMutation = { redeliverWebhookDelivery: { id: string, status: Types.WebhookQueueStatus, nextAttemptAt: string } };


export const WebhooksDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"Webhooks"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"webhooks"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"targetUrl"}},{"kind":"Field","name":{"kind":"Name","value":"kind"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"eventTypes"}},{"kind":"Field","name":{"kind":"Name","value":"failurePolicy"}},{"kind":"Field","name":{"kind":"Name","value":"circuit"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"state"}}]}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"channel"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}}]}}]}}]} as unknown as DocumentNode<WebhooksQuery, WebhooksQueryVariables>;
export const WebhookDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"Webhook"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"webhook"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"targetUrl"}},{"kind":"Field","name":{"kind":"Name","value":"kind"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"eventTypes"}},{"kind":"Field","name":{"kind":"Name","value":"failurePolicy"}},{"kind":"Field","name":{"kind":"Name","value":"timeoutMs"}},{"kind":"Field","name":{"kind":"Name","value":"customHeaders"}},{"kind":"Field","name":{"kind":"Name","value":"payloadVersion"}},{"kind":"Field","name":{"kind":"Name","value":"circuit"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"state"}},{"kind":"Field","name":{"kind":"Name","value":"consecutiveFailures"}},{"kind":"Field","name":{"kind":"Name","value":"openedAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"channel"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}}]}}]}}]} as unknown as DocumentNode<WebhookQuery, WebhookQueryVariables>;
export const WebhookEventTypesDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"WebhookEventTypes"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"webhookEventTypes"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"kind"}}]}}]}}]} as unknown as DocumentNode<WebhookEventTypesQuery, WebhookEventTypesQueryVariables>;
export const WebhookDeliveryStatsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"WebhookDeliveryStats"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"webhookId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"webhookDeliveryStats"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"webhookId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"webhookId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"total"}},{"kind":"Field","name":{"kind":"Name","value":"succeeded"}},{"kind":"Field","name":{"kind":"Name","value":"failed"}},{"kind":"Field","name":{"kind":"Name","value":"successRate"}},{"kind":"Field","name":{"kind":"Name","value":"avgSuccessMs"}},{"kind":"Field","name":{"kind":"Name","value":"lastDeliveryAt"}},{"kind":"Field","name":{"kind":"Name","value":"lastSuccessAt"}},{"kind":"Field","name":{"kind":"Name","value":"lastFailureAt"}}]}}]}}]} as unknown as DocumentNode<WebhookDeliveryStatsQuery, WebhookDeliveryStatsQueryVariables>;
export const WebhookDeliveriesDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"WebhookDeliveries"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"webhookId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"webhookDeliveries"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"webhookId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"webhookId"}}},{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"eventType"}},{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"requestBody"}},{"kind":"Field","name":{"kind":"Name","value":"requestHeaders"}},{"kind":"Field","name":{"kind":"Name","value":"responseStatus"}},{"kind":"Field","name":{"kind":"Name","value":"responseBody"}},{"kind":"Field","name":{"kind":"Name","value":"responseHeaders"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"success"}},{"kind":"Field","name":{"kind":"Name","value":"errorMessage"}},{"kind":"Field","name":{"kind":"Name","value":"attempt"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}}]}}]}}]} as unknown as DocumentNode<WebhookDeliveriesQuery, WebhookDeliveriesQueryVariables>;
export const CreateWebhookDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateWebhook"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"CreateWebhookInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createWebhook"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"rawSecret"}},{"kind":"Field","name":{"kind":"Name","value":"webhook"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"targetUrl"}},{"kind":"Field","name":{"kind":"Name","value":"kind"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"eventTypes"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"channel"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}}]}}]}}]}}]} as unknown as DocumentNode<CreateWebhookMutation, CreateWebhookMutationVariables>;
export const UpdateWebhookDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateWebhook"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateWebhookInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateWebhook"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"targetUrl"}},{"kind":"Field","name":{"kind":"Name","value":"eventTypes"}},{"kind":"Field","name":{"kind":"Name","value":"failurePolicy"}},{"kind":"Field","name":{"kind":"Name","value":"timeoutMs"}},{"kind":"Field","name":{"kind":"Name","value":"customHeaders"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"channel"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}}]}}]}}]} as unknown as DocumentNode<UpdateWebhookMutation, UpdateWebhookMutationVariables>;
export const SetWebhookActiveDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SetWebhookActive"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"active"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Boolean"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"setWebhookActive"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}},{"kind":"Argument","name":{"kind":"Name","value":"active"},"value":{"kind":"Variable","name":{"kind":"Name","value":"active"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}}]}}]}}]} as unknown as DocumentNode<SetWebhookActiveMutation, SetWebhookActiveMutationVariables>;
export const RotateWebhookSecretDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RotateWebhookSecret"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"rotateWebhookSecret"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"rawSecret"}},{"kind":"Field","name":{"kind":"Name","value":"webhook"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]}}]} as unknown as DocumentNode<RotateWebhookSecretMutation, RotateWebhookSecretMutationVariables>;
export const DeleteWebhookDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteWebhook"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteWebhook"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}]}]}}]} as unknown as DocumentNode<DeleteWebhookMutation, DeleteWebhookMutationVariables>;
export const ClearWebhookDeliveriesDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"ClearWebhookDeliveries"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"webhookId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"clearWebhookDeliveries"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"webhookId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"webhookId"}}}]}]}}]} as unknown as DocumentNode<ClearWebhookDeliveriesMutation, ClearWebhookDeliveriesMutationVariables>;
export const WebhookDeadLettersDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"WebhookDeadLetters"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"webhookId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"webhookDeadLetters"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"webhookId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"webhookId"}}},{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"eventType"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"attempts"}},{"kind":"Field","name":{"kind":"Name","value":"nextAttemptAt"}},{"kind":"Field","name":{"kind":"Name","value":"lastError"}},{"kind":"Field","name":{"kind":"Name","value":"payload"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]}}]} as unknown as DocumentNode<WebhookDeadLettersQuery, WebhookDeadLettersQueryVariables>;
export const RetryWebhookDeadLetterDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RetryWebhookDeadLetter"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"retryWebhookDeadLetter"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"attempts"}},{"kind":"Field","name":{"kind":"Name","value":"nextAttemptAt"}}]}}]}}]} as unknown as DocumentNode<RetryWebhookDeadLetterMutation, RetryWebhookDeadLetterMutationVariables>;
export const ClearWebhookDeadLettersDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"ClearWebhookDeadLetters"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"webhookId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"clearWebhookDeadLetters"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"webhookId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"webhookId"}}}]}]}}]} as unknown as DocumentNode<ClearWebhookDeadLettersMutation, ClearWebhookDeadLettersMutationVariables>;
export const RedeliverWebhookDeliveryDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RedeliverWebhookDelivery"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"deliveryId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"redeliverWebhookDelivery"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"deliveryId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"deliveryId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"nextAttemptAt"}}]}}]}}]} as unknown as DocumentNode<RedeliverWebhookDeliveryMutation, RedeliverWebhookDeliveryMutationVariables>;