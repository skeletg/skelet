/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
import type * as Types from '../../graphql/schema.types';

import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type CreateLocaleInput = {
  code: string;
  dateFormat?: string | null | undefined;
  decimalSeparator?: string | null | undefined;
  defaultCountryCode?: string | null | undefined;
  description?: string | null | undefined;
  emailFromNameOverride?: string | null | undefined;
  firstDayOfWeek?: number | null | undefined;
  flagEmoji?: string | null | undefined;
  hreflangValue?: string | null | undefined;
  isActive?: boolean | null | undefined;
  isRTL?: boolean | null | undefined;
  metadata?: Array<MetadataInput> | null | undefined;
  name: string;
  nativeName?: string | null | undefined;
  pluralRule?: string | null | undefined;
  script?: string | null | undefined;
  sortOrder?: number | null | undefined;
  thousandsSeparator?: string | null | undefined;
  timeFormat?: string | null | undefined;
};

export type MetadataInput = {
  key: string;
  value: string;
};

export type UpdateLocaleInput = {
  code: string;
  dateFormat?: string | null | undefined;
  decimalSeparator?: string | null | undefined;
  defaultCountryCode?: string | null | undefined;
  description?: string | null | undefined;
  emailFromNameOverride?: string | null | undefined;
  firstDayOfWeek?: number | null | undefined;
  flagEmoji?: string | null | undefined;
  hreflangValue?: string | null | undefined;
  isActive?: boolean | null | undefined;
  isRTL?: boolean | null | undefined;
  metadata?: Array<MetadataInput> | null | undefined;
  name?: string | null | undefined;
  nativeName?: string | null | undefined;
  pluralRule?: string | null | undefined;
  script?: string | null | undefined;
  sortOrder?: number | null | undefined;
  thousandsSeparator?: string | null | undefined;
  timeFormat?: string | null | undefined;
};

export type LocalesPickerQueryVariables = Exact<{ [key: string]: never; }>;


export type LocalesPickerQuery = { locales: Array<{ code: string, name: string, nativeName: string, flagEmoji: string }> };

export type LocalesAdminListQueryVariables = Exact<{ [key: string]: never; }>;


export type LocalesAdminListQuery = { locales: Array<{ code: string, name: string, nativeName: string, isActive: boolean, isRTL: boolean, sortOrder: number, flagEmoji: string, defaultCountryCode: string, updatedAt: string }> };

export type LocaleQueryVariables = Exact<{
  code: string;
}>;


export type LocaleQuery = { locale: { code: string, name: string, nativeName: string, description: string, isActive: boolean, isRTL: boolean, sortOrder: number, flagEmoji: string, dateFormat: string, timeFormat: string, decimalSeparator: string, thousandsSeparator: string, firstDayOfWeek: number, defaultCountryCode: string, hreflangValue: string, script: string, pluralRule: string, emailFromNameOverride: string, createdAt: string, updatedAt: string, metadata: Array<{ key: string, value: string }> } };

export type CreateLocaleMutationVariables = Exact<{
  input: Types.CreateLocaleInput;
}>;


export type CreateLocaleMutation = { createLocale: { code: string, name: string, nativeName: string, isActive: boolean, isRTL: boolean, sortOrder: number, flagEmoji: string } };

export type UpdateLocaleMutationVariables = Exact<{
  input: Types.UpdateLocaleInput;
}>;


export type UpdateLocaleMutation = { updateLocale: { code: string, name: string, nativeName: string, isActive: boolean, isRTL: boolean, sortOrder: number, flagEmoji: string, defaultCountryCode: string, updatedAt: string } };

export type DeleteLocaleMutationVariables = Exact<{
  code: string;
}>;


export type DeleteLocaleMutation = { deleteLocale: boolean };

export type SetChannelLocalesMutationVariables = Exact<{
  channelId: string | number;
  codes: Array<string> | string;
}>;


export type SetChannelLocalesMutation = { setChannelLocales: { id: string, locales: Array<{ code: string, name: string, nativeName: string, flagEmoji: string, isActive: boolean, isRTL: boolean, sortOrder: number }> } };


export const LocalesPickerDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"LocalesPicker"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"locales"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"activeOnly"},"value":{"kind":"BooleanValue","value":true}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"nativeName"}},{"kind":"Field","name":{"kind":"Name","value":"flagEmoji"}}]}}]}}]} as unknown as DocumentNode<LocalesPickerQuery, LocalesPickerQueryVariables>;
export const LocalesAdminListDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"LocalesAdminList"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"locales"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"activeOnly"},"value":{"kind":"BooleanValue","value":false}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"nativeName"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"isRTL"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}},{"kind":"Field","name":{"kind":"Name","value":"flagEmoji"}},{"kind":"Field","name":{"kind":"Name","value":"defaultCountryCode"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]}}]} as unknown as DocumentNode<LocalesAdminListQuery, LocalesAdminListQueryVariables>;
export const LocaleDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"Locale"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"code"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"locale"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"code"},"value":{"kind":"Variable","name":{"kind":"Name","value":"code"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"nativeName"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"isRTL"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}},{"kind":"Field","name":{"kind":"Name","value":"flagEmoji"}},{"kind":"Field","name":{"kind":"Name","value":"dateFormat"}},{"kind":"Field","name":{"kind":"Name","value":"timeFormat"}},{"kind":"Field","name":{"kind":"Name","value":"decimalSeparator"}},{"kind":"Field","name":{"kind":"Name","value":"thousandsSeparator"}},{"kind":"Field","name":{"kind":"Name","value":"firstDayOfWeek"}},{"kind":"Field","name":{"kind":"Name","value":"defaultCountryCode"}},{"kind":"Field","name":{"kind":"Name","value":"hreflangValue"}},{"kind":"Field","name":{"kind":"Name","value":"script"}},{"kind":"Field","name":{"kind":"Name","value":"pluralRule"}},{"kind":"Field","name":{"kind":"Name","value":"emailFromNameOverride"}},{"kind":"Field","name":{"kind":"Name","value":"metadata"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"key"}},{"kind":"Field","name":{"kind":"Name","value":"value"}}]}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]}}]} as unknown as DocumentNode<LocaleQuery, LocaleQueryVariables>;
export const CreateLocaleDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateLocale"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"CreateLocaleInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createLocale"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"nativeName"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"isRTL"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}},{"kind":"Field","name":{"kind":"Name","value":"flagEmoji"}}]}}]}}]} as unknown as DocumentNode<CreateLocaleMutation, CreateLocaleMutationVariables>;
export const UpdateLocaleDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateLocale"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateLocaleInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateLocale"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"nativeName"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"isRTL"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}},{"kind":"Field","name":{"kind":"Name","value":"flagEmoji"}},{"kind":"Field","name":{"kind":"Name","value":"defaultCountryCode"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]}}]} as unknown as DocumentNode<UpdateLocaleMutation, UpdateLocaleMutationVariables>;
export const DeleteLocaleDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteLocale"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"code"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteLocale"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"code"},"value":{"kind":"Variable","name":{"kind":"Name","value":"code"}}}]}]}}]} as unknown as DocumentNode<DeleteLocaleMutation, DeleteLocaleMutationVariables>;
export const SetChannelLocalesDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SetChannelLocales"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"codes"}},"type":{"kind":"NonNullType","type":{"kind":"ListType","type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"setChannelLocales"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"channelId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}}},{"kind":"Argument","name":{"kind":"Name","value":"codes"},"value":{"kind":"Variable","name":{"kind":"Name","value":"codes"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"locales"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"nativeName"}},{"kind":"Field","name":{"kind":"Name","value":"flagEmoji"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"isRTL"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}}]}}]}}]}}]} as unknown as DocumentNode<SetChannelLocalesMutation, SetChannelLocalesMutationVariables>;