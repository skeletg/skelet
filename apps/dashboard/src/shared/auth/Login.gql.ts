/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
import type * as Types from '../../graphql/schema.types';

import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type AdminLoginInput = {
  email: string;
  password: string;
  totpCode?: string | null | undefined;
};

export type AdminLoginMutationVariables = Exact<{
  input: Types.AdminLoginInput;
}>;


export type AdminLoginMutation = { adminLogin: { accessToken: string, refreshToken: string, firstLogin: boolean, admin: { id: string, email: string, firstName: string, lastName: string, isSuperuser: boolean, roles: Array<{ id: string, slug: string, name: string }> } } };

export type AdminSsoOptionQueryVariables = Exact<{ [key: string]: never; }>;


export type AdminSsoOptionQuery = { adminSsoOption: { label: string } | null };

export type AdminSsoStartMutationVariables = Exact<{
  redirectUri: string;
}>;


export type AdminSsoStartMutation = { adminSsoStart: { authorizeUrl: string, state: string, binding: string } };

export type AdminSsoCompleteMutationVariables = Exact<{
  state: string;
  code: string;
  binding: string;
}>;


export type AdminSsoCompleteMutation = { adminSsoComplete: { accessToken: string, refreshToken: string, firstLogin: boolean, admin: { id: string, email: string, firstName: string, lastName: string, isSuperuser: boolean, roles: Array<{ id: string, slug: string, name: string }> } } };


export const AdminLoginDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"AdminLogin"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"AdminLoginInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"adminLogin"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"accessToken"}},{"kind":"Field","name":{"kind":"Name","value":"refreshToken"}},{"kind":"Field","name":{"kind":"Name","value":"firstLogin"}},{"kind":"Field","name":{"kind":"Name","value":"admin"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"email"}},{"kind":"Field","name":{"kind":"Name","value":"firstName"}},{"kind":"Field","name":{"kind":"Name","value":"lastName"}},{"kind":"Field","name":{"kind":"Name","value":"isSuperuser"}},{"kind":"Field","name":{"kind":"Name","value":"roles"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}}]}}]}}]}}]} as unknown as DocumentNode<AdminLoginMutation, AdminLoginMutationVariables>;
export const AdminSsoOptionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"AdminSsoOption"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"adminSsoOption"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"label"}}]}}]}}]} as unknown as DocumentNode<AdminSsoOptionQuery, AdminSsoOptionQueryVariables>;
export const AdminSsoStartDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"AdminSsoStart"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"redirectUri"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"adminSsoStart"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"redirectUri"},"value":{"kind":"Variable","name":{"kind":"Name","value":"redirectUri"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"authorizeUrl"}},{"kind":"Field","name":{"kind":"Name","value":"state"}},{"kind":"Field","name":{"kind":"Name","value":"binding"}}]}}]}}]} as unknown as DocumentNode<AdminSsoStartMutation, AdminSsoStartMutationVariables>;
export const AdminSsoCompleteDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"AdminSsoComplete"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"state"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"code"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"binding"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"adminSsoComplete"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"state"},"value":{"kind":"Variable","name":{"kind":"Name","value":"state"}}},{"kind":"Argument","name":{"kind":"Name","value":"code"},"value":{"kind":"Variable","name":{"kind":"Name","value":"code"}}},{"kind":"Argument","name":{"kind":"Name","value":"binding"},"value":{"kind":"Variable","name":{"kind":"Name","value":"binding"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"accessToken"}},{"kind":"Field","name":{"kind":"Name","value":"refreshToken"}},{"kind":"Field","name":{"kind":"Name","value":"firstLogin"}},{"kind":"Field","name":{"kind":"Name","value":"admin"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"email"}},{"kind":"Field","name":{"kind":"Name","value":"firstName"}},{"kind":"Field","name":{"kind":"Name","value":"lastName"}},{"kind":"Field","name":{"kind":"Name","value":"isSuperuser"}},{"kind":"Field","name":{"kind":"Name","value":"roles"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}}]}}]}}]}}]} as unknown as DocumentNode<AdminSsoCompleteMutation, AdminSsoCompleteMutationVariables>;