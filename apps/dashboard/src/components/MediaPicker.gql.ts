/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
import type * as Types from '../graphql/schema.types';

import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type MediaMimeGroup =
  | 'DOCUMENT'
  | 'IMAGE';

export type MediaPickerSearchQueryVariables = Exact<{
  query?: string | null | undefined;
  mimeGroup?: Types.MediaMimeGroup | null | undefined;
  limit?: number | null | undefined;
}>;


export type MediaPickerSearchQuery = { searchMediaAssets: { totalCount: number, items: Array<{ id: string, url: string, filename: string, title: string, altText: string, mimeType: string, width: number | null, height: number | null }> } };

export type MediaPickerAssetQueryVariables = Exact<{
  id: string | number;
}>;


export type MediaPickerAssetQuery = { mediaAsset: { id: string, url: string, filename: string, title: string, altText: string } };


export const MediaPickerSearchDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"MediaPickerSearch"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"query"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"mimeGroup"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"MediaMimeGroup"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"searchMediaAssets"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"query"},"value":{"kind":"Variable","name":{"kind":"Name","value":"query"}}},{"kind":"Argument","name":{"kind":"Name","value":"mimeGroup"},"value":{"kind":"Variable","name":{"kind":"Name","value":"mimeGroup"}}},{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"totalCount"}},{"kind":"Field","name":{"kind":"Name","value":"items"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"altText"}},{"kind":"Field","name":{"kind":"Name","value":"mimeType"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}}]}}]}}]}}]} as unknown as DocumentNode<MediaPickerSearchQuery, MediaPickerSearchQueryVariables>;
export const MediaPickerAssetDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"MediaPickerAsset"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"mediaAsset"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"altText"}}]}}]}}]} as unknown as DocumentNode<MediaPickerAssetQuery, MediaPickerAssetQueryVariables>;