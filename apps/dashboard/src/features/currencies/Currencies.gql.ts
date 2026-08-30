/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
import type * as Types from '../../graphql/schema.types';

import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type CreateCurrencyInput = {
  code: string;
  decimals?: number | null | undefined;
  isActive?: boolean | null | undefined;
  metadata?: Array<MetadataInput> | null | undefined;
  name: string;
  sortOrder?: number | null | undefined;
  symbol?: string | null | undefined;
  symbolPosition?: string | null | undefined;
};

export type MetadataInput = {
  key: string;
  value: string;
};

export type UpdateCurrencyInput = {
  code: string;
  decimals?: number | null | undefined;
  exchangeRate?: number | null | undefined;
  isActive?: boolean | null | undefined;
  metadata?: Array<MetadataInput> | null | undefined;
  name?: string | null | undefined;
  sortOrder?: number | null | undefined;
  symbol?: string | null | undefined;
  symbolPosition?: string | null | undefined;
};

export type CurrenciesPickerQueryVariables = Exact<{ [key: string]: never; }>;


export type CurrenciesPickerQuery = { currencies: Array<{ code: string, name: string, symbol: string, decimals: number, symbolPosition: string }> };

export type CurrenciesAdminListQueryVariables = Exact<{ [key: string]: never; }>;


export type CurrenciesAdminListQuery = { currencies: Array<{ code: string, name: string, symbol: string, decimals: number, symbolPosition: string, isActive: boolean, sortOrder: number, exchangeRate: number | null, rateUpdatedAt: string | null, updatedAt: string, metadata: Array<{ key: string, value: string }> }> };

export type CurrencyQueryVariables = Exact<{
  code: string;
}>;


export type CurrencyQuery = { currency: { code: string, name: string, symbol: string, decimals: number, symbolPosition: string, isActive: boolean, sortOrder: number, exchangeRate: number | null, rateUpdatedAt: string | null, createdAt: string, updatedAt: string, metadata: Array<{ key: string, value: string }> } };

export type CreateCurrencyMutationVariables = Exact<{
  input: Types.CreateCurrencyInput;
}>;


export type CreateCurrencyMutation = { createCurrency: { code: string, name: string, symbol: string, decimals: number, symbolPosition: string, isActive: boolean, sortOrder: number } };

export type UpdateCurrencyMutationVariables = Exact<{
  input: Types.UpdateCurrencyInput;
}>;


export type UpdateCurrencyMutation = { updateCurrency: { code: string, name: string, symbol: string, decimals: number, symbolPosition: string, isActive: boolean, sortOrder: number, updatedAt: string } };

export type DeleteCurrencyMutationVariables = Exact<{
  code: string;
}>;


export type DeleteCurrencyMutation = { deleteCurrency: boolean };


export const CurrenciesPickerDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"CurrenciesPicker"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"currencies"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"activeOnly"},"value":{"kind":"BooleanValue","value":true}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"symbol"}},{"kind":"Field","name":{"kind":"Name","value":"decimals"}},{"kind":"Field","name":{"kind":"Name","value":"symbolPosition"}}]}}]}}]} as unknown as DocumentNode<CurrenciesPickerQuery, CurrenciesPickerQueryVariables>;
export const CurrenciesAdminListDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"CurrenciesAdminList"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"currencies"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"activeOnly"},"value":{"kind":"BooleanValue","value":false}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"symbol"}},{"kind":"Field","name":{"kind":"Name","value":"decimals"}},{"kind":"Field","name":{"kind":"Name","value":"symbolPosition"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}},{"kind":"Field","name":{"kind":"Name","value":"metadata"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"key"}},{"kind":"Field","name":{"kind":"Name","value":"value"}}]}},{"kind":"Field","name":{"kind":"Name","value":"exchangeRate"}},{"kind":"Field","name":{"kind":"Name","value":"rateUpdatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]}}]} as unknown as DocumentNode<CurrenciesAdminListQuery, CurrenciesAdminListQueryVariables>;
export const CurrencyDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"Currency"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"code"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"currency"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"code"},"value":{"kind":"Variable","name":{"kind":"Name","value":"code"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"symbol"}},{"kind":"Field","name":{"kind":"Name","value":"decimals"}},{"kind":"Field","name":{"kind":"Name","value":"symbolPosition"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}},{"kind":"Field","name":{"kind":"Name","value":"metadata"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"key"}},{"kind":"Field","name":{"kind":"Name","value":"value"}}]}},{"kind":"Field","name":{"kind":"Name","value":"exchangeRate"}},{"kind":"Field","name":{"kind":"Name","value":"rateUpdatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]}}]} as unknown as DocumentNode<CurrencyQuery, CurrencyQueryVariables>;
export const CreateCurrencyDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateCurrency"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"CreateCurrencyInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createCurrency"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"symbol"}},{"kind":"Field","name":{"kind":"Name","value":"decimals"}},{"kind":"Field","name":{"kind":"Name","value":"symbolPosition"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}}]}}]}}]} as unknown as DocumentNode<CreateCurrencyMutation, CreateCurrencyMutationVariables>;
export const UpdateCurrencyDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateCurrency"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateCurrencyInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateCurrency"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"symbol"}},{"kind":"Field","name":{"kind":"Name","value":"decimals"}},{"kind":"Field","name":{"kind":"Name","value":"symbolPosition"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]}}]} as unknown as DocumentNode<UpdateCurrencyMutation, UpdateCurrencyMutationVariables>;
export const DeleteCurrencyDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteCurrency"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"code"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteCurrency"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"code"},"value":{"kind":"Variable","name":{"kind":"Name","value":"code"}}}]}]}}]} as unknown as DocumentNode<DeleteCurrencyMutation, DeleteCurrencyMutationVariables>;