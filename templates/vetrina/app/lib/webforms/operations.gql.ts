/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
import type * as Types from '../../graphql/schema.types';

import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type SubmitWebFormInput = {
  captchaToken?: string | null | undefined;
  files?: Array<WebFormFileInput> | null | undefined;
  formId: string | number;
  honeypot?: string | null | undefined;
  values: unknown;
};

export type WebFormFileInput = {
  fieldKey: string;
  name: string;
  objectKey: string;
};

export type SubmitWebFormMutationVariables = Exact<{
  input: Types.SubmitWebFormInput;
}>;


export type SubmitWebFormMutation = { submitWebForm: { ok: boolean, successMessage: string | null, redirectUrl: string | null, errors: Array<{ field: string, message: string }> } };

export type PresignWebFormUploadMutationVariables = Exact<{
  formId: string | number;
  fieldKey: string;
  filename: string;
  contentType: string;
  sizeBytes: number;
}>;


export type PresignWebFormUploadMutation = { presignWebFormUpload: { url: string, objectKey: string } };


export const SubmitWebFormDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SubmitWebForm"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"SubmitWebFormInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"submitWebForm"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"ok"}},{"kind":"Field","name":{"kind":"Name","value":"successMessage"}},{"kind":"Field","name":{"kind":"Name","value":"redirectUrl"}},{"kind":"Field","name":{"kind":"Name","value":"errors"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"field"}},{"kind":"Field","name":{"kind":"Name","value":"message"}}]}}]}}]}}]} as unknown as DocumentNode<SubmitWebFormMutation, SubmitWebFormMutationVariables>;
export const PresignWebFormUploadDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"PresignWebFormUpload"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"formId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"fieldKey"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"filename"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"contentType"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"sizeBytes"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"presignWebFormUpload"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"formId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"formId"}}},{"kind":"Argument","name":{"kind":"Name","value":"fieldKey"},"value":{"kind":"Variable","name":{"kind":"Name","value":"fieldKey"}}},{"kind":"Argument","name":{"kind":"Name","value":"filename"},"value":{"kind":"Variable","name":{"kind":"Name","value":"filename"}}},{"kind":"Argument","name":{"kind":"Name","value":"contentType"},"value":{"kind":"Variable","name":{"kind":"Name","value":"contentType"}}},{"kind":"Argument","name":{"kind":"Name","value":"sizeBytes"},"value":{"kind":"Variable","name":{"kind":"Name","value":"sizeBytes"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"objectKey"}}]}}]}}]} as unknown as DocumentNode<PresignWebFormUploadMutation, PresignWebFormUploadMutationVariables>;