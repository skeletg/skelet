/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
import type * as Types from '../../graphql/schema.types';

import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type BusinessIdFieldInput = {
  hint?: string | null | undefined;
  key?: string | null | undefined;
  label: string;
  pattern?: string | null | undefined;
  required: boolean;
};

export type CreateCountryInput = {
  businessIdFields?: Array<BusinessIdFieldInput> | null | undefined;
  code: string;
  defaultCurrencyCode?: string | null | undefined;
  emojiFlag?: string | null | undefined;
  isActive?: boolean | null | undefined;
  metadata?: Array<MetadataInput> | null | undefined;
  name: string;
  nativeName?: string | null | undefined;
  phoneCode?: string | null | undefined;
  sortOrder?: number | null | undefined;
};

export type MetadataInput = {
  key: string;
  value: string;
};

export type UpdateCountryInput = {
  businessIdFields?: Array<BusinessIdFieldInput> | null | undefined;
  code: string;
  defaultCurrencyCode?: string | null | undefined;
  emojiFlag?: string | null | undefined;
  isActive?: boolean | null | undefined;
  metadata?: Array<MetadataInput> | null | undefined;
  name?: string | null | undefined;
  nativeName?: string | null | undefined;
  phoneCode?: string | null | undefined;
  sortOrder?: number | null | undefined;
};

export type CountriesPickerQueryVariables = Exact<{ [key: string]: never; }>;


export type CountriesPickerQuery = { countries: Array<{ code: string, name: string, emojiFlag: string, phoneCode: string, defaultCurrency: { code: string } | null }> };

export type CountriesAdminListQueryVariables = Exact<{ [key: string]: never; }>;


export type CountriesAdminListQuery = { countries: Array<{ code: string, name: string, nativeName: string, emojiFlag: string, phoneCode: string, defaultCurrencyCode: string, isActive: boolean, sortOrder: number, updatedAt: string, defaultCurrency: { code: string, symbol: string } | null }> };

export type CountryQueryVariables = Exact<{
  code: string;
}>;


export type CountryQuery = { country: { code: string, name: string, nativeName: string, emojiFlag: string, phoneCode: string, iso3Code: string, numericCode: string, defaultCurrencyCode: string, isActive: boolean, sortOrder: number, createdAt: string, updatedAt: string, defaultCurrency: { code: string, symbol: string } | null, metadata: Array<{ key: string, value: string }>, businessIdFields: Array<{ key: string, label: string, required: boolean, pattern: string, hint: string }>, regions: Array<{ code: string, name: string, isActive: boolean, sortOrder: number }> } };

export type CreateCountryMutationVariables = Exact<{
  input: Types.CreateCountryInput;
}>;


export type CreateCountryMutation = { createCountry: { code: string, name: string, nativeName: string, emojiFlag: string, phoneCode: string, defaultCurrencyCode: string, isActive: boolean, sortOrder: number } };

export type UpdateCountryMutationVariables = Exact<{
  input: Types.UpdateCountryInput;
}>;


export type UpdateCountryMutation = { updateCountry: { code: string, name: string, nativeName: string, emojiFlag: string, phoneCode: string, defaultCurrencyCode: string, isActive: boolean, sortOrder: number, updatedAt: string } };

export type DeleteCountryMutationVariables = Exact<{
  code: string;
}>;


export type DeleteCountryMutation = { deleteCountry: boolean };


export const CountriesPickerDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"CountriesPicker"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"countries"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"activeOnly"},"value":{"kind":"BooleanValue","value":true}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"emojiFlag"}},{"kind":"Field","name":{"kind":"Name","value":"phoneCode"}},{"kind":"Field","name":{"kind":"Name","value":"defaultCurrency"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}}]}}]}}]}}]} as unknown as DocumentNode<CountriesPickerQuery, CountriesPickerQueryVariables>;
export const CountriesAdminListDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"CountriesAdminList"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"countries"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"activeOnly"},"value":{"kind":"BooleanValue","value":false}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"nativeName"}},{"kind":"Field","name":{"kind":"Name","value":"emojiFlag"}},{"kind":"Field","name":{"kind":"Name","value":"phoneCode"}},{"kind":"Field","name":{"kind":"Name","value":"defaultCurrencyCode"}},{"kind":"Field","name":{"kind":"Name","value":"defaultCurrency"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"symbol"}}]}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]}}]} as unknown as DocumentNode<CountriesAdminListQuery, CountriesAdminListQueryVariables>;
export const CountryDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"Country"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"code"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"country"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"code"},"value":{"kind":"Variable","name":{"kind":"Name","value":"code"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"nativeName"}},{"kind":"Field","name":{"kind":"Name","value":"emojiFlag"}},{"kind":"Field","name":{"kind":"Name","value":"phoneCode"}},{"kind":"Field","name":{"kind":"Name","value":"iso3Code"}},{"kind":"Field","name":{"kind":"Name","value":"numericCode"}},{"kind":"Field","name":{"kind":"Name","value":"defaultCurrencyCode"}},{"kind":"Field","name":{"kind":"Name","value":"defaultCurrency"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"symbol"}}]}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}},{"kind":"Field","name":{"kind":"Name","value":"metadata"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"key"}},{"kind":"Field","name":{"kind":"Name","value":"value"}}]}},{"kind":"Field","name":{"kind":"Name","value":"businessIdFields"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"key"}},{"kind":"Field","name":{"kind":"Name","value":"label"}},{"kind":"Field","name":{"kind":"Name","value":"required"}},{"kind":"Field","name":{"kind":"Name","value":"pattern"}},{"kind":"Field","name":{"kind":"Name","value":"hint"}}]}},{"kind":"Field","name":{"kind":"Name","value":"regions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}}]}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]}}]} as unknown as DocumentNode<CountryQuery, CountryQueryVariables>;
export const CreateCountryDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateCountry"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"CreateCountryInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createCountry"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"nativeName"}},{"kind":"Field","name":{"kind":"Name","value":"emojiFlag"}},{"kind":"Field","name":{"kind":"Name","value":"phoneCode"}},{"kind":"Field","name":{"kind":"Name","value":"defaultCurrencyCode"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}}]}}]}}]} as unknown as DocumentNode<CreateCountryMutation, CreateCountryMutationVariables>;
export const UpdateCountryDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateCountry"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateCountryInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateCountry"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"nativeName"}},{"kind":"Field","name":{"kind":"Name","value":"emojiFlag"}},{"kind":"Field","name":{"kind":"Name","value":"phoneCode"}},{"kind":"Field","name":{"kind":"Name","value":"defaultCurrencyCode"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]}}]} as unknown as DocumentNode<UpdateCountryMutation, UpdateCountryMutationVariables>;
export const DeleteCountryDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteCountry"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"code"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteCountry"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"code"},"value":{"kind":"Variable","name":{"kind":"Name","value":"code"}}}]}]}}]} as unknown as DocumentNode<DeleteCountryMutation, DeleteCountryMutationVariables>;