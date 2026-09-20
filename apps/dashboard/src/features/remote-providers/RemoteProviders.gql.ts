/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
import type * as Types from '../../graphql/schema.types';

import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type CreateRemoteProviderInput = {
  cacheTtlS?: number | null | undefined;
  channelId?: string | number | null | undefined;
  config?: unknown;
  failurePolicy?: RemoteProviderFailurePolicy | null | undefined;
  kind: RemoteProviderKind;
  name: string;
  slug: string;
  targetUrl: string;
  timeoutMs?: number | null | undefined;
};

export type RemoteProviderFailurePolicy =
  | 'FAIL_CLOSED'
  | 'FAIL_OPEN';

export type RemoteProviderKind =
  | 'ADDRESS_VALIDATOR'
  | 'CURRENCY_RATE'
  | 'CUSTOMER_SEGMENTS'
  | 'FRAUD_RISK'
  | 'INVENTORY_AVAILABILITY'
  | 'PRICING'
  | 'PROMOTION_ACTION'
  | 'PROMOTION_CONDITION'
  | 'RECOMMENDATION'
  | 'RETURNS_POLICY'
  | 'REVIEWS'
  | 'SEARCH'
  | 'SHIPPING_CARRIER'
  | 'SHIPPING_TRACKING'
  | 'TAX_PROVIDER';

export type UpdateRemoteProviderInput = {
  cacheTtlS?: number | null | undefined;
  config?: unknown;
  failurePolicy?: RemoteProviderFailurePolicy | null | undefined;
  id: string | number;
  name?: string | null | undefined;
  targetUrl?: string | null | undefined;
  timeoutMs?: number | null | undefined;
};

export type WebhookCircuitState =
  | 'CLOSED'
  | 'HALF_OPEN'
  | 'OPEN';

export type RemoteProvidersQueryVariables = Exact<{ [key: string]: never; }>;


export type RemoteProvidersQuery = { remoteProviders: Array<{ id: string, kind: Types.RemoteProviderKind, slug: string, name: string, targetUrl: string, config: unknown, timeoutMs: number, cacheTtlS: number, failurePolicy: Types.RemoteProviderFailurePolicy, isActive: boolean, createdAt: string, updatedAt: string, circuit: { state: Types.WebhookCircuitState, consecutiveFailures: number }, channel: { id: string, slug: string, name: string } | null }> };

export type CreateRemoteProviderMutationVariables = Exact<{
  input: Types.CreateRemoteProviderInput;
}>;


export type CreateRemoteProviderMutation = { createRemoteProvider: { rawSecret: string, provider: { id: string, name: string } } };

export type UpdateRemoteProviderMutationVariables = Exact<{
  input: Types.UpdateRemoteProviderInput;
}>;


export type UpdateRemoteProviderMutation = { updateRemoteProvider: { id: string, name: string, targetUrl: string, config: unknown, timeoutMs: number, cacheTtlS: number, failurePolicy: Types.RemoteProviderFailurePolicy, updatedAt: string } };

export type SetRemoteProviderActiveMutationVariables = Exact<{
  id: string | number;
  active: boolean;
}>;


export type SetRemoteProviderActiveMutation = { setRemoteProviderActive: { id: string, isActive: boolean } };

export type RotateRemoteProviderSecretMutationVariables = Exact<{
  id: string | number;
}>;


export type RotateRemoteProviderSecretMutation = { rotateRemoteProviderSecret: { rawSecret: string, provider: { id: string, name: string } } };

export type DeleteRemoteProviderMutationVariables = Exact<{
  id: string | number;
}>;


export type DeleteRemoteProviderMutation = { deleteRemoteProvider: boolean };


export const RemoteProvidersDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"RemoteProviders"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"remoteProviders"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"kind"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"targetUrl"}},{"kind":"Field","name":{"kind":"Name","value":"config"}},{"kind":"Field","name":{"kind":"Name","value":"timeoutMs"}},{"kind":"Field","name":{"kind":"Name","value":"cacheTtlS"}},{"kind":"Field","name":{"kind":"Name","value":"failurePolicy"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"circuit"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"state"}},{"kind":"Field","name":{"kind":"Name","value":"consecutiveFailures"}}]}},{"kind":"Field","name":{"kind":"Name","value":"channel"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]}}]} as unknown as DocumentNode<RemoteProvidersQuery, RemoteProvidersQueryVariables>;
export const CreateRemoteProviderDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateRemoteProvider"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"CreateRemoteProviderInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createRemoteProvider"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"rawSecret"}},{"kind":"Field","name":{"kind":"Name","value":"provider"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}}]}}]}}]} as unknown as DocumentNode<CreateRemoteProviderMutation, CreateRemoteProviderMutationVariables>;
export const UpdateRemoteProviderDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateRemoteProvider"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateRemoteProviderInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateRemoteProvider"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"targetUrl"}},{"kind":"Field","name":{"kind":"Name","value":"config"}},{"kind":"Field","name":{"kind":"Name","value":"timeoutMs"}},{"kind":"Field","name":{"kind":"Name","value":"cacheTtlS"}},{"kind":"Field","name":{"kind":"Name","value":"failurePolicy"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]}}]} as unknown as DocumentNode<UpdateRemoteProviderMutation, UpdateRemoteProviderMutationVariables>;
export const SetRemoteProviderActiveDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SetRemoteProviderActive"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"active"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Boolean"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"setRemoteProviderActive"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}},{"kind":"Argument","name":{"kind":"Name","value":"active"},"value":{"kind":"Variable","name":{"kind":"Name","value":"active"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}}]}}]}}]} as unknown as DocumentNode<SetRemoteProviderActiveMutation, SetRemoteProviderActiveMutationVariables>;
export const RotateRemoteProviderSecretDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RotateRemoteProviderSecret"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"rotateRemoteProviderSecret"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"rawSecret"}},{"kind":"Field","name":{"kind":"Name","value":"provider"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}}]}}]}}]} as unknown as DocumentNode<RotateRemoteProviderSecretMutation, RotateRemoteProviderSecretMutationVariables>;
export const DeleteRemoteProviderDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteRemoteProvider"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteRemoteProvider"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}]}]}}]} as unknown as DocumentNode<DeleteRemoteProviderMutation, DeleteRemoteProviderMutationVariables>;