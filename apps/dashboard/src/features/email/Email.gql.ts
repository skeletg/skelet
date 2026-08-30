/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
import type * as Types from '../../graphql/schema.types';

import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type SaveEmailBlockInput = {
  blocks: unknown;
  channelId: string | number;
  css?: string | null | undefined;
  fields?: unknown;
  htmlTemplate?: string | null | undefined;
  icon?: string | null | undefined;
  name: string;
  scripted?: boolean | null | undefined;
  slug: string;
  textTemplate?: string | null | undefined;
};

export type EmailBlockFieldsFragment = { channelId: string, slug: string, name: string, icon: string, fields: unknown, blocks: unknown, htmlTemplate: string, textTemplate: string, css: string, scripted: boolean };

export type EmailBlocksQueryVariables = Exact<{
  channelId: string | number;
}>;


export type EmailBlocksQuery = { emailBlocks: Array<{ channelId: string, slug: string, name: string, icon: string, fields: unknown, blocks: unknown, htmlTemplate: string, textTemplate: string, css: string, scripted: boolean }> };

export type SaveEmailBlockMutationVariables = Exact<{
  input: Types.SaveEmailBlockInput;
}>;


export type SaveEmailBlockMutation = { saveEmailBlock: { channelId: string, slug: string, name: string, icon: string, fields: unknown, blocks: unknown, htmlTemplate: string, textTemplate: string, css: string, scripted: boolean } };

export type DeleteEmailBlockMutationVariables = Exact<{
  channelId: string | number;
  slug: string;
}>;


export type DeleteEmailBlockMutation = { deleteEmailBlock: boolean };

export const EmailBlockFieldsFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"EmailBlockFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"EmailBlockSnippet"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"channelId"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"icon"}},{"kind":"Field","name":{"kind":"Name","value":"fields"}},{"kind":"Field","name":{"kind":"Name","value":"blocks"}},{"kind":"Field","name":{"kind":"Name","value":"htmlTemplate"}},{"kind":"Field","name":{"kind":"Name","value":"textTemplate"}},{"kind":"Field","name":{"kind":"Name","value":"css"}},{"kind":"Field","name":{"kind":"Name","value":"scripted"}}]}}]} as unknown as DocumentNode<EmailBlockFieldsFragment, unknown>;
export const EmailBlocksDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"EmailBlocks"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"emailBlocks"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"channelId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"EmailBlockFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"EmailBlockFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"EmailBlockSnippet"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"channelId"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"icon"}},{"kind":"Field","name":{"kind":"Name","value":"fields"}},{"kind":"Field","name":{"kind":"Name","value":"blocks"}},{"kind":"Field","name":{"kind":"Name","value":"htmlTemplate"}},{"kind":"Field","name":{"kind":"Name","value":"textTemplate"}},{"kind":"Field","name":{"kind":"Name","value":"css"}},{"kind":"Field","name":{"kind":"Name","value":"scripted"}}]}}]} as unknown as DocumentNode<EmailBlocksQuery, EmailBlocksQueryVariables>;
export const SaveEmailBlockDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SaveEmailBlock"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"SaveEmailBlockInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"saveEmailBlock"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"EmailBlockFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"EmailBlockFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"EmailBlockSnippet"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"channelId"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"icon"}},{"kind":"Field","name":{"kind":"Name","value":"fields"}},{"kind":"Field","name":{"kind":"Name","value":"blocks"}},{"kind":"Field","name":{"kind":"Name","value":"htmlTemplate"}},{"kind":"Field","name":{"kind":"Name","value":"textTemplate"}},{"kind":"Field","name":{"kind":"Name","value":"css"}},{"kind":"Field","name":{"kind":"Name","value":"scripted"}}]}}]} as unknown as DocumentNode<SaveEmailBlockMutation, SaveEmailBlockMutationVariables>;
export const DeleteEmailBlockDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteEmailBlock"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"slug"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteEmailBlock"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"channelId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}}},{"kind":"Argument","name":{"kind":"Name","value":"slug"},"value":{"kind":"Variable","name":{"kind":"Name","value":"slug"}}}]}]}}]} as unknown as DocumentNode<DeleteEmailBlockMutation, DeleteEmailBlockMutationVariables>;