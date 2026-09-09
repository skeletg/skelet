/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
import type * as Types from '../../graphql/signals.types';

import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type EventDestinationInput = {
  channelId: string | number;
  eventNames?: Array<string> | null | undefined;
  eventSources?: Array<string> | null | undefined;
  isActive?: boolean | null | undefined;
  kind: string;
  name: string;
  secret?: string | null | undefined;
  targetUrl: string;
};

export type EventDestinationsQueryVariables = Exact<{
  channelId: string | number;
}>;


export type EventDestinationsQuery = { eventDestinations: Array<{ id: string, channelId: string, name: string, kind: string, targetUrl: string, eventSources: Array<string>, eventNames: Array<string>, isActive: boolean, lastError: string, lastForwardedAt: string | null, createdAt: string, updatedAt: string }> };

export type CreateEventDestinationMutationVariables = Exact<{
  input: Types.EventDestinationInput;
}>;


export type CreateEventDestinationMutation = { createEventDestination: { id: string } };

export type UpdateEventDestinationMutationVariables = Exact<{
  id: string | number;
  input: Types.EventDestinationInput;
}>;


export type UpdateEventDestinationMutation = { updateEventDestination: { id: string, name: string, kind: string, targetUrl: string, eventSources: Array<string>, eventNames: Array<string>, isActive: boolean, updatedAt: string } };

export type DeleteEventDestinationMutationVariables = Exact<{
  id: string | number;
}>;


export type DeleteEventDestinationMutation = { deleteEventDestination: boolean };

export type SyncAudienceMutationVariables = Exact<{
  segmentId: string | number;
  destinationId: string | number;
}>;


export type SyncAudienceMutation = { syncAudience: number };


export const EventDestinationsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"EventDestinations"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"eventDestinations"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"channelId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"channelId"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"kind"}},{"kind":"Field","name":{"kind":"Name","value":"targetUrl"}},{"kind":"Field","name":{"kind":"Name","value":"eventSources"}},{"kind":"Field","name":{"kind":"Name","value":"eventNames"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"lastError"}},{"kind":"Field","name":{"kind":"Name","value":"lastForwardedAt"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]}}]} as unknown as DocumentNode<EventDestinationsQuery, EventDestinationsQueryVariables>;
export const CreateEventDestinationDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateEventDestination"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"EventDestinationInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createEventDestination"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]} as unknown as DocumentNode<CreateEventDestinationMutation, CreateEventDestinationMutationVariables>;
export const UpdateEventDestinationDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateEventDestination"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"EventDestinationInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateEventDestination"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}},{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"kind"}},{"kind":"Field","name":{"kind":"Name","value":"targetUrl"}},{"kind":"Field","name":{"kind":"Name","value":"eventSources"}},{"kind":"Field","name":{"kind":"Name","value":"eventNames"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]}}]} as unknown as DocumentNode<UpdateEventDestinationMutation, UpdateEventDestinationMutationVariables>;
export const DeleteEventDestinationDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteEventDestination"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteEventDestination"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}]}]}}]} as unknown as DocumentNode<DeleteEventDestinationMutation, DeleteEventDestinationMutationVariables>;
export const SyncAudienceDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SyncAudience"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"segmentId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"destinationId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"syncAudience"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"segmentId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"segmentId"}}},{"kind":"Argument","name":{"kind":"Name","value":"destinationId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"destinationId"}}}]}]}}]} as unknown as DocumentNode<SyncAudienceMutation, SyncAudienceMutationVariables>;