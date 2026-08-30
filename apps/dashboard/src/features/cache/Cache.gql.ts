/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };

import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type CacheSurfaceRowFragment = { key: string, label: string, enabled: boolean, ttlSeconds: number, bulkInvalidatable: boolean };

export type CacheRevalidationRowFragment = { id: string, surface: string, tags: Array<string>, paths: Array<string>, reason: string, actorId: string | null, createdAt: string };

export type CacheSurfacesQueryVariables = Exact<{ [key: string]: never; }>;


export type CacheSurfacesQuery = { cacheSurfaces: Array<{ key: string, label: string, enabled: boolean, ttlSeconds: number, bulkInvalidatable: boolean }> };

export type CacheRevalidationsQueryVariables = Exact<{
  limit?: number | null | undefined;
}>;


export type CacheRevalidationsQuery = { cacheRevalidations: Array<{ id: string, surface: string, tags: Array<string>, paths: Array<string>, reason: string, actorId: string | null, createdAt: string }> };

export type PurgeCacheMutationVariables = Exact<{
  surfaceKey?: string | null | undefined;
  tags?: Array<string> | string | null | undefined;
  paths?: Array<string> | string | null | undefined;
}>;


export type PurgeCacheMutation = { purgeCache: boolean };

export type RebuildCacheMutationVariables = Exact<{
  paths: Array<string> | string;
}>;


export type RebuildCacheMutation = { rebuildCache: boolean };

export type RebuildTaskRowFragment = { id: string, surface: string, path: string, status: string, attempts: number, lastError: string, reason: string, updatedAt: string };

export type RebuildQueueQueryVariables = Exact<{
  recentLimit?: number | null | undefined;
}>;


export type RebuildQueueQuery = { rebuildQueue: { pending: number, running: number, done: number, failed: number, recent: Array<{ id: string, surface: string, path: string, status: string, attempts: number, lastError: string, reason: string, updatedAt: string }> } };

export type EnqueueRebuildMutationVariables = Exact<{
  paths: Array<string> | string;
}>;


export type EnqueueRebuildMutation = { enqueueRebuild: number };

export const CacheSurfaceRowFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"CacheSurfaceRow"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CacheSurface"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"key"}},{"kind":"Field","name":{"kind":"Name","value":"label"}},{"kind":"Field","name":{"kind":"Name","value":"enabled"}},{"kind":"Field","name":{"kind":"Name","value":"ttlSeconds"}},{"kind":"Field","name":{"kind":"Name","value":"bulkInvalidatable"}}]}}]} as unknown as DocumentNode<CacheSurfaceRowFragment, unknown>;
export const CacheRevalidationRowFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"CacheRevalidationRow"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CacheRevalidation"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"surface"}},{"kind":"Field","name":{"kind":"Name","value":"tags"}},{"kind":"Field","name":{"kind":"Name","value":"paths"}},{"kind":"Field","name":{"kind":"Name","value":"reason"}},{"kind":"Field","name":{"kind":"Name","value":"actorId"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}}]}}]} as unknown as DocumentNode<CacheRevalidationRowFragment, unknown>;
export const RebuildTaskRowFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"RebuildTaskRow"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RebuildTask"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"surface"}},{"kind":"Field","name":{"kind":"Name","value":"path"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"attempts"}},{"kind":"Field","name":{"kind":"Name","value":"lastError"}},{"kind":"Field","name":{"kind":"Name","value":"reason"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]} as unknown as DocumentNode<RebuildTaskRowFragment, unknown>;
export const CacheSurfacesDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"CacheSurfaces"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"cacheSurfaces"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"CacheSurfaceRow"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"CacheSurfaceRow"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CacheSurface"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"key"}},{"kind":"Field","name":{"kind":"Name","value":"label"}},{"kind":"Field","name":{"kind":"Name","value":"enabled"}},{"kind":"Field","name":{"kind":"Name","value":"ttlSeconds"}},{"kind":"Field","name":{"kind":"Name","value":"bulkInvalidatable"}}]}}]} as unknown as DocumentNode<CacheSurfacesQuery, CacheSurfacesQueryVariables>;
export const CacheRevalidationsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"CacheRevalidations"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"cacheRevalidations"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"CacheRevalidationRow"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"CacheRevalidationRow"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CacheRevalidation"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"surface"}},{"kind":"Field","name":{"kind":"Name","value":"tags"}},{"kind":"Field","name":{"kind":"Name","value":"paths"}},{"kind":"Field","name":{"kind":"Name","value":"reason"}},{"kind":"Field","name":{"kind":"Name","value":"actorId"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}}]}}]} as unknown as DocumentNode<CacheRevalidationsQuery, CacheRevalidationsQueryVariables>;
export const PurgeCacheDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"PurgeCache"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"surfaceKey"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"tags"}},"type":{"kind":"ListType","type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"paths"}},"type":{"kind":"ListType","type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"purgeCache"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"surfaceKey"},"value":{"kind":"Variable","name":{"kind":"Name","value":"surfaceKey"}}},{"kind":"Argument","name":{"kind":"Name","value":"tags"},"value":{"kind":"Variable","name":{"kind":"Name","value":"tags"}}},{"kind":"Argument","name":{"kind":"Name","value":"paths"},"value":{"kind":"Variable","name":{"kind":"Name","value":"paths"}}}]}]}}]} as unknown as DocumentNode<PurgeCacheMutation, PurgeCacheMutationVariables>;
export const RebuildCacheDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RebuildCache"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"paths"}},"type":{"kind":"NonNullType","type":{"kind":"ListType","type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"rebuildCache"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"paths"},"value":{"kind":"Variable","name":{"kind":"Name","value":"paths"}}}]}]}}]} as unknown as DocumentNode<RebuildCacheMutation, RebuildCacheMutationVariables>;
export const RebuildQueueDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"RebuildQueue"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"recentLimit"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"rebuildQueue"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"recentLimit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"recentLimit"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"pending"}},{"kind":"Field","name":{"kind":"Name","value":"running"}},{"kind":"Field","name":{"kind":"Name","value":"done"}},{"kind":"Field","name":{"kind":"Name","value":"failed"}},{"kind":"Field","name":{"kind":"Name","value":"recent"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"RebuildTaskRow"}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"RebuildTaskRow"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"RebuildTask"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"surface"}},{"kind":"Field","name":{"kind":"Name","value":"path"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"attempts"}},{"kind":"Field","name":{"kind":"Name","value":"lastError"}},{"kind":"Field","name":{"kind":"Name","value":"reason"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]} as unknown as DocumentNode<RebuildQueueQuery, RebuildQueueQueryVariables>;
export const EnqueueRebuildDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"EnqueueRebuild"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"paths"}},"type":{"kind":"NonNullType","type":{"kind":"ListType","type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"enqueueRebuild"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"paths"},"value":{"kind":"Variable","name":{"kind":"Name","value":"paths"}}}]}]}}]} as unknown as DocumentNode<EnqueueRebuildMutation, EnqueueRebuildMutationVariables>;