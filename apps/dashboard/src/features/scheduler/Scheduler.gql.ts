/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
import type * as Types from '../../graphql/schema.types';

import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type ConfigScopeInput = {
  channelId?: string | number | null | undefined;
  kind: ConfigScopeKind;
  locale?: string | null | undefined;
};

export type ConfigScopeKind =
  | 'CHANNEL'
  | 'DEFAULT';

export type CreateWebhookJobInput = {
  alertAfterFailures?: number | null | undefined;
  body?: string | null | undefined;
  description?: string | null | undefined;
  enabled?: boolean | null | undefined;
  headers?: Array<HttpHeaderInput> | null | undefined;
  maxAttempts?: number | null | undefined;
  method?: string | null | undefined;
  name: string;
  retryBackoffSeconds?: number | null | undefined;
  schedule: string;
  secret?: string | null | undefined;
  timezone?: string | null | undefined;
  url: string;
};

export type HttpHeaderInput = {
  name: string;
  value: string;
};

export type UpdateScheduledJobInput = {
  alertAfterFailures?: number | null | undefined;
  enabled: boolean;
  id: string | number;
  maxAttempts?: number | null | undefined;
  retryBackoffSeconds?: number | null | undefined;
  schedule: string;
  timeoutSeconds?: number | null | undefined;
  timezone?: string | null | undefined;
};

export type UpdateWebhookJobInput = {
  body?: string | null | undefined;
  description?: string | null | undefined;
  headers?: Array<HttpHeaderInput> | null | undefined;
  id: string | number;
  method?: string | null | undefined;
  secret?: string | null | undefined;
  url: string;
};

export type ScheduledJobsQueryVariables = Exact<{ [key: string]: never; }>;


export type ScheduledJobsQuery = { scheduledJobs: Array<{ id: string, name: string, kind: string, schedule: string, enabled: boolean, description: string, timezone: string, maxAttempts: number, retryBackoffSeconds: number, currentAttempt: number, alertAfterFailures: number, consecutiveFailures: number, timeoutSeconds: number, nextRunAt: string | null, lastRunAt: string | null, lastStatus: string, lastError: string, lastDurationMs: number | null, createdAt: string, updatedAt: string, webhook: { host: string, method: string, hasSecret: boolean } | null }> };

export type ScheduledJobRunsQueryVariables = Exact<{
  jobId: string | number;
  limit?: number | null | undefined;
}>;


export type ScheduledJobRunsQuery = { scheduledJobRuns: Array<{ startedAt: string, finishedAt: string, status: string, error: string, durationMs: number, attempt: number }> };

export type PreviewScheduleQueryVariables = Exact<{
  schedule: string;
  timezone?: string | null | undefined;
  count?: number | null | undefined;
}>;


export type PreviewScheduleQuery = { previewSchedule: { valid: boolean, error: string, nextRuns: Array<string> } };

export type WebhookJobConfigQueryVariables = Exact<{
  id: string | number;
}>;


export type WebhookJobConfigQuery = { webhookJobConfig: { url: string, method: string, body: string, hasSecret: boolean, headers: Array<{ name: string, value: string }> } };

export type SchedulerConfigQueryVariables = Exact<{
  scope?: Types.ConfigScopeInput | null | undefined;
}>;


export type SchedulerConfigQuery = { enabled: { value: string, isDefault: boolean }, alertWebhookUrl: { value: string, isDefault: boolean, isSet: boolean }, historyRetentionDays: { value: string, isDefault: boolean } };

export type UpdateScheduledJobMutationVariables = Exact<{
  input: Types.UpdateScheduledJobInput;
}>;


export type UpdateScheduledJobMutation = { updateScheduledJob: { id: string, schedule: string, enabled: boolean, timezone: string, maxAttempts: number, retryBackoffSeconds: number, alertAfterFailures: number, timeoutSeconds: number, nextRunAt: string | null } };

export type CreateWebhookJobMutationVariables = Exact<{
  input: Types.CreateWebhookJobInput;
}>;


export type CreateWebhookJobMutation = { createWebhookJob: { id: string, name: string } };

export type UpdateWebhookJobMutationVariables = Exact<{
  input: Types.UpdateWebhookJobInput;
}>;


export type UpdateWebhookJobMutation = { updateWebhookJob: { id: string, description: string, webhook: { host: string, method: string, hasSecret: boolean } | null } };

export type DeleteScheduledJobMutationVariables = Exact<{
  id: string | number;
}>;


export type DeleteScheduledJobMutation = { deleteScheduledJob: boolean };

export type RunScheduledJobNowMutationVariables = Exact<{
  id: string | number;
}>;


export type RunScheduledJobNowMutation = { runScheduledJobNow: { id: string, lastStatus: string, lastRunAt: string | null, nextRunAt: string | null } };


export const ScheduledJobsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ScheduledJobs"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"scheduledJobs"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"kind"}},{"kind":"Field","name":{"kind":"Name","value":"schedule"}},{"kind":"Field","name":{"kind":"Name","value":"enabled"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"timezone"}},{"kind":"Field","name":{"kind":"Name","value":"maxAttempts"}},{"kind":"Field","name":{"kind":"Name","value":"retryBackoffSeconds"}},{"kind":"Field","name":{"kind":"Name","value":"currentAttempt"}},{"kind":"Field","name":{"kind":"Name","value":"alertAfterFailures"}},{"kind":"Field","name":{"kind":"Name","value":"consecutiveFailures"}},{"kind":"Field","name":{"kind":"Name","value":"timeoutSeconds"}},{"kind":"Field","name":{"kind":"Name","value":"webhook"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"host"}},{"kind":"Field","name":{"kind":"Name","value":"method"}},{"kind":"Field","name":{"kind":"Name","value":"hasSecret"}}]}},{"kind":"Field","name":{"kind":"Name","value":"nextRunAt"}},{"kind":"Field","name":{"kind":"Name","value":"lastRunAt"}},{"kind":"Field","name":{"kind":"Name","value":"lastStatus"}},{"kind":"Field","name":{"kind":"Name","value":"lastError"}},{"kind":"Field","name":{"kind":"Name","value":"lastDurationMs"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]}}]} as unknown as DocumentNode<ScheduledJobsQuery, ScheduledJobsQueryVariables>;
export const ScheduledJobRunsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ScheduledJobRuns"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"jobId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"scheduledJobRuns"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"jobId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"jobId"}}},{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"startedAt"}},{"kind":"Field","name":{"kind":"Name","value":"finishedAt"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"error"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"attempt"}}]}}]}}]} as unknown as DocumentNode<ScheduledJobRunsQuery, ScheduledJobRunsQueryVariables>;
export const PreviewScheduleDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"PreviewSchedule"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"schedule"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"timezone"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"count"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"previewSchedule"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"schedule"},"value":{"kind":"Variable","name":{"kind":"Name","value":"schedule"}}},{"kind":"Argument","name":{"kind":"Name","value":"timezone"},"value":{"kind":"Variable","name":{"kind":"Name","value":"timezone"}}},{"kind":"Argument","name":{"kind":"Name","value":"count"},"value":{"kind":"Variable","name":{"kind":"Name","value":"count"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"valid"}},{"kind":"Field","name":{"kind":"Name","value":"error"}},{"kind":"Field","name":{"kind":"Name","value":"nextRuns"}}]}}]}}]} as unknown as DocumentNode<PreviewScheduleQuery, PreviewScheduleQueryVariables>;
export const WebhookJobConfigDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"WebhookJobConfig"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"webhookJobConfig"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"method"}},{"kind":"Field","name":{"kind":"Name","value":"body"}},{"kind":"Field","name":{"kind":"Name","value":"headers"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"value"}}]}},{"kind":"Field","name":{"kind":"Name","value":"hasSecret"}}]}}]}}]} as unknown as DocumentNode<WebhookJobConfigQuery, WebhookJobConfigQueryVariables>;
export const SchedulerConfigDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"SchedulerConfig"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"scope"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"ConfigScopeInput"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","alias":{"kind":"Name","value":"enabled"},"name":{"kind":"Name","value":"configValue"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"path"},"value":{"kind":"StringValue","value":"system/scheduler/enabled","block":false}},{"kind":"Argument","name":{"kind":"Name","value":"scope"},"value":{"kind":"Variable","name":{"kind":"Name","value":"scope"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"value"}},{"kind":"Field","name":{"kind":"Name","value":"isDefault"}}]}},{"kind":"Field","alias":{"kind":"Name","value":"alertWebhookUrl"},"name":{"kind":"Name","value":"configValue"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"path"},"value":{"kind":"StringValue","value":"system/scheduler/alert_webhook_url","block":false}},{"kind":"Argument","name":{"kind":"Name","value":"scope"},"value":{"kind":"Variable","name":{"kind":"Name","value":"scope"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"value"}},{"kind":"Field","name":{"kind":"Name","value":"isDefault"}},{"kind":"Field","name":{"kind":"Name","value":"isSet"}}]}},{"kind":"Field","alias":{"kind":"Name","value":"historyRetentionDays"},"name":{"kind":"Name","value":"configValue"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"path"},"value":{"kind":"StringValue","value":"system/scheduler/history_retention_days","block":false}},{"kind":"Argument","name":{"kind":"Name","value":"scope"},"value":{"kind":"Variable","name":{"kind":"Name","value":"scope"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"value"}},{"kind":"Field","name":{"kind":"Name","value":"isDefault"}}]}}]}}]} as unknown as DocumentNode<SchedulerConfigQuery, SchedulerConfigQueryVariables>;
export const UpdateScheduledJobDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateScheduledJob"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateScheduledJobInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateScheduledJob"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"schedule"}},{"kind":"Field","name":{"kind":"Name","value":"enabled"}},{"kind":"Field","name":{"kind":"Name","value":"timezone"}},{"kind":"Field","name":{"kind":"Name","value":"maxAttempts"}},{"kind":"Field","name":{"kind":"Name","value":"retryBackoffSeconds"}},{"kind":"Field","name":{"kind":"Name","value":"alertAfterFailures"}},{"kind":"Field","name":{"kind":"Name","value":"timeoutSeconds"}},{"kind":"Field","name":{"kind":"Name","value":"nextRunAt"}}]}}]}}]} as unknown as DocumentNode<UpdateScheduledJobMutation, UpdateScheduledJobMutationVariables>;
export const CreateWebhookJobDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateWebhookJob"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"CreateWebhookJobInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createWebhookJob"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}}]}}]} as unknown as DocumentNode<CreateWebhookJobMutation, CreateWebhookJobMutationVariables>;
export const UpdateWebhookJobDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateWebhookJob"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateWebhookJobInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateWebhookJob"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"webhook"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"host"}},{"kind":"Field","name":{"kind":"Name","value":"method"}},{"kind":"Field","name":{"kind":"Name","value":"hasSecret"}}]}}]}}]}}]} as unknown as DocumentNode<UpdateWebhookJobMutation, UpdateWebhookJobMutationVariables>;
export const DeleteScheduledJobDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteScheduledJob"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteScheduledJob"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}]}]}}]} as unknown as DocumentNode<DeleteScheduledJobMutation, DeleteScheduledJobMutationVariables>;
export const RunScheduledJobNowDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RunScheduledJobNow"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"runScheduledJobNow"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"lastStatus"}},{"kind":"Field","name":{"kind":"Name","value":"lastRunAt"}},{"kind":"Field","name":{"kind":"Name","value":"nextRunAt"}}]}}]}}]} as unknown as DocumentNode<RunScheduledJobNowMutation, RunScheduledJobNowMutationVariables>;