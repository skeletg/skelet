/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
import type * as Types from '../../graphql/schema.types';

import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type CreateShippingMethodInput = {
  allowedCountries?: Array<string> | null | undefined;
  calcType: ShippingCalcType;
  code: string;
  countryMode?: string | null | undefined;
  flatCents?: number | null | undefined;
  freeAboveCents?: number | null | undefined;
  isActive?: boolean | null | undefined;
  name?: string | null | undefined;
  sortOrder?: number | null | undefined;
  zoneId: string | number;
};

export type CreateShippingZoneInput = {
  channelId: string | number;
  code: string;
  countries?: Array<string> | null | undefined;
  isActive?: boolean | null | undefined;
  name?: string | null | undefined;
  sortOrder?: number | null | undefined;
};

export type ShippingBoxInput = {
  boxWeightGrams?: number | null | undefined;
  channelId: string | number;
  code: string;
  innerHeightMm: number;
  innerLengthMm: number;
  innerWidthMm: number;
  isActive?: boolean | null | undefined;
  maxWeightGrams?: number | null | undefined;
  name?: string | null | undefined;
  sortOrder?: number | null | undefined;
};

export type ShippingCalcType =
  | 'FLAT'
  | 'FREE'
  | 'FREE_ABOVE';

export type UpdateShippingMethodInput = {
  allowedCountries?: Array<string> | null | undefined;
  calcType?: ShippingCalcType | null | undefined;
  code?: string | null | undefined;
  countryMode?: string | null | undefined;
  flatCents?: number | null | undefined;
  freeAboveCents?: number | null | undefined;
  id: string | number;
  isActive?: boolean | null | undefined;
  name?: string | null | undefined;
  sortOrder?: number | null | undefined;
};

export type UpdateShippingZoneInput = {
  code?: string | null | undefined;
  countries?: Array<string> | null | undefined;
  id: string | number;
  isActive?: boolean | null | undefined;
  name?: string | null | undefined;
  sortOrder?: number | null | undefined;
};

export type ShippingMethodFieldsFragment = { id: string, zoneId: string, code: string, name: string, calcType: Types.ShippingCalcType, flatCents: number, freeAboveCents: number, sortOrder: number, isActive: boolean, countryMode: string, allowedCountries: Array<string>, createdAt: string, updatedAt: string };

export type ShippingZoneFieldsFragment = { id: string, channelId: string, code: string, name: string, sortOrder: number, isActive: boolean, countries: Array<string>, createdAt: string, updatedAt: string, methods: Array<{ id: string, zoneId: string, code: string, name: string, calcType: Types.ShippingCalcType, flatCents: number, freeAboveCents: number, sortOrder: number, isActive: boolean, countryMode: string, allowedCountries: Array<string>, createdAt: string, updatedAt: string }> };

export type ShippingZonesQueryVariables = Exact<{
  channelId: string | number;
}>;


export type ShippingZonesQuery = { shippingZones: Array<{ id: string, channelId: string, code: string, name: string, sortOrder: number, isActive: boolean, countries: Array<string>, createdAt: string, updatedAt: string, methods: Array<{ id: string, zoneId: string, code: string, name: string, calcType: Types.ShippingCalcType, flatCents: number, freeAboveCents: number, sortOrder: number, isActive: boolean, countryMode: string, allowedCountries: Array<string>, createdAt: string, updatedAt: string }> }> };

export type ShippingZoneQueryVariables = Exact<{
  id: string | number;
}>;


export type ShippingZoneQuery = { shippingZone: { id: string, channelId: string, code: string, name: string, sortOrder: number, isActive: boolean, countries: Array<string>, createdAt: string, updatedAt: string, methods: Array<{ id: string, zoneId: string, code: string, name: string, calcType: Types.ShippingCalcType, flatCents: number, freeAboveCents: number, sortOrder: number, isActive: boolean, countryMode: string, allowedCountries: Array<string>, createdAt: string, updatedAt: string }> } | null };

export type CreateShippingZoneMutationVariables = Exact<{
  input: Types.CreateShippingZoneInput;
}>;


export type CreateShippingZoneMutation = { createShippingZone: { id: string, channelId: string, code: string, name: string, sortOrder: number, isActive: boolean, countries: Array<string>, createdAt: string, updatedAt: string, methods: Array<{ id: string, zoneId: string, code: string, name: string, calcType: Types.ShippingCalcType, flatCents: number, freeAboveCents: number, sortOrder: number, isActive: boolean, countryMode: string, allowedCountries: Array<string>, createdAt: string, updatedAt: string }> } };

export type UpdateShippingZoneMutationVariables = Exact<{
  input: Types.UpdateShippingZoneInput;
}>;


export type UpdateShippingZoneMutation = { updateShippingZone: { id: string, channelId: string, code: string, name: string, sortOrder: number, isActive: boolean, countries: Array<string>, createdAt: string, updatedAt: string, methods: Array<{ id: string, zoneId: string, code: string, name: string, calcType: Types.ShippingCalcType, flatCents: number, freeAboveCents: number, sortOrder: number, isActive: boolean, countryMode: string, allowedCountries: Array<string>, createdAt: string, updatedAt: string }> } };

export type DeleteShippingZoneMutationVariables = Exact<{
  id: string | number;
}>;


export type DeleteShippingZoneMutation = { deleteShippingZone: boolean };

export type CreateShippingMethodMutationVariables = Exact<{
  input: Types.CreateShippingMethodInput;
}>;


export type CreateShippingMethodMutation = { createShippingMethod: { id: string, zoneId: string, code: string, name: string, calcType: Types.ShippingCalcType, flatCents: number, freeAboveCents: number, sortOrder: number, isActive: boolean, countryMode: string, allowedCountries: Array<string>, createdAt: string, updatedAt: string } };

export type UpdateShippingMethodMutationVariables = Exact<{
  input: Types.UpdateShippingMethodInput;
}>;


export type UpdateShippingMethodMutation = { updateShippingMethod: { id: string, zoneId: string, code: string, name: string, calcType: Types.ShippingCalcType, flatCents: number, freeAboveCents: number, sortOrder: number, isActive: boolean, countryMode: string, allowedCountries: Array<string>, createdAt: string, updatedAt: string } };

export type DeleteShippingMethodMutationVariables = Exact<{
  id: string | number;
}>;


export type DeleteShippingMethodMutation = { deleteShippingMethod: boolean };

export type ShippingBoxesQueryVariables = Exact<{
  channelId: string | number;
}>;


export type ShippingBoxesQuery = { shippingBoxes: Array<{ id: string, channelId: string, code: string, name: string, innerLengthMm: number, innerWidthMm: number, innerHeightMm: number, maxWeightGrams: number, boxWeightGrams: number, isActive: boolean, sortOrder: number }> };

export type UpsertShippingBoxMutationVariables = Exact<{
  input: Types.ShippingBoxInput;
}>;


export type UpsertShippingBoxMutation = { upsertShippingBox: { id: string } };

export type DeleteShippingBoxMutationVariables = Exact<{
  id: string | number;
}>;


export type DeleteShippingBoxMutation = { deleteShippingBox: boolean };

export const ShippingMethodFieldsFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ShippingMethodFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ShippingMethod"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"zoneId"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"calcType"}},{"kind":"Field","name":{"kind":"Name","value":"flatCents"}},{"kind":"Field","name":{"kind":"Name","value":"freeAboveCents"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"countryMode"}},{"kind":"Field","name":{"kind":"Name","value":"allowedCountries"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]} as unknown as DocumentNode<ShippingMethodFieldsFragment, unknown>;
export const ShippingZoneFieldsFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ShippingZoneFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ShippingZone"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"channelId"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"countries"}},{"kind":"Field","name":{"kind":"Name","value":"methods"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ShippingMethodFields"}}]}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ShippingMethodFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ShippingMethod"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"zoneId"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"calcType"}},{"kind":"Field","name":{"kind":"Name","value":"flatCents"}},{"kind":"Field","name":{"kind":"Name","value":"freeAboveCents"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"countryMode"}},{"kind":"Field","name":{"kind":"Name","value":"allowedCountries"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]} as unknown as DocumentNode<ShippingZoneFieldsFragment, unknown>;
export const ShippingZonesDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ShippingZones"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"shippingZones"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"channelId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ShippingZoneFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ShippingMethodFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ShippingMethod"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"zoneId"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"calcType"}},{"kind":"Field","name":{"kind":"Name","value":"flatCents"}},{"kind":"Field","name":{"kind":"Name","value":"freeAboveCents"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"countryMode"}},{"kind":"Field","name":{"kind":"Name","value":"allowedCountries"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ShippingZoneFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ShippingZone"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"channelId"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"countries"}},{"kind":"Field","name":{"kind":"Name","value":"methods"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ShippingMethodFields"}}]}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]} as unknown as DocumentNode<ShippingZonesQuery, ShippingZonesQueryVariables>;
export const ShippingZoneDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ShippingZone"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"shippingZone"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ShippingZoneFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ShippingMethodFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ShippingMethod"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"zoneId"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"calcType"}},{"kind":"Field","name":{"kind":"Name","value":"flatCents"}},{"kind":"Field","name":{"kind":"Name","value":"freeAboveCents"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"countryMode"}},{"kind":"Field","name":{"kind":"Name","value":"allowedCountries"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ShippingZoneFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ShippingZone"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"channelId"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"countries"}},{"kind":"Field","name":{"kind":"Name","value":"methods"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ShippingMethodFields"}}]}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]} as unknown as DocumentNode<ShippingZoneQuery, ShippingZoneQueryVariables>;
export const CreateShippingZoneDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateShippingZone"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"CreateShippingZoneInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createShippingZone"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ShippingZoneFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ShippingMethodFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ShippingMethod"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"zoneId"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"calcType"}},{"kind":"Field","name":{"kind":"Name","value":"flatCents"}},{"kind":"Field","name":{"kind":"Name","value":"freeAboveCents"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"countryMode"}},{"kind":"Field","name":{"kind":"Name","value":"allowedCountries"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ShippingZoneFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ShippingZone"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"channelId"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"countries"}},{"kind":"Field","name":{"kind":"Name","value":"methods"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ShippingMethodFields"}}]}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]} as unknown as DocumentNode<CreateShippingZoneMutation, CreateShippingZoneMutationVariables>;
export const UpdateShippingZoneDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateShippingZone"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateShippingZoneInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateShippingZone"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ShippingZoneFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ShippingMethodFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ShippingMethod"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"zoneId"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"calcType"}},{"kind":"Field","name":{"kind":"Name","value":"flatCents"}},{"kind":"Field","name":{"kind":"Name","value":"freeAboveCents"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"countryMode"}},{"kind":"Field","name":{"kind":"Name","value":"allowedCountries"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ShippingZoneFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ShippingZone"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"channelId"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"countries"}},{"kind":"Field","name":{"kind":"Name","value":"methods"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ShippingMethodFields"}}]}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]} as unknown as DocumentNode<UpdateShippingZoneMutation, UpdateShippingZoneMutationVariables>;
export const DeleteShippingZoneDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteShippingZone"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteShippingZone"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}]}]}}]} as unknown as DocumentNode<DeleteShippingZoneMutation, DeleteShippingZoneMutationVariables>;
export const CreateShippingMethodDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateShippingMethod"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"CreateShippingMethodInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createShippingMethod"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ShippingMethodFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ShippingMethodFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ShippingMethod"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"zoneId"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"calcType"}},{"kind":"Field","name":{"kind":"Name","value":"flatCents"}},{"kind":"Field","name":{"kind":"Name","value":"freeAboveCents"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"countryMode"}},{"kind":"Field","name":{"kind":"Name","value":"allowedCountries"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]} as unknown as DocumentNode<CreateShippingMethodMutation, CreateShippingMethodMutationVariables>;
export const UpdateShippingMethodDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateShippingMethod"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateShippingMethodInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateShippingMethod"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ShippingMethodFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ShippingMethodFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ShippingMethod"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"zoneId"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"calcType"}},{"kind":"Field","name":{"kind":"Name","value":"flatCents"}},{"kind":"Field","name":{"kind":"Name","value":"freeAboveCents"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"countryMode"}},{"kind":"Field","name":{"kind":"Name","value":"allowedCountries"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]} as unknown as DocumentNode<UpdateShippingMethodMutation, UpdateShippingMethodMutationVariables>;
export const DeleteShippingMethodDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteShippingMethod"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteShippingMethod"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}]}]}}]} as unknown as DocumentNode<DeleteShippingMethodMutation, DeleteShippingMethodMutationVariables>;
export const ShippingBoxesDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ShippingBoxes"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"shippingBoxes"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"channelId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"channelId"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"innerLengthMm"}},{"kind":"Field","name":{"kind":"Name","value":"innerWidthMm"}},{"kind":"Field","name":{"kind":"Name","value":"innerHeightMm"}},{"kind":"Field","name":{"kind":"Name","value":"maxWeightGrams"}},{"kind":"Field","name":{"kind":"Name","value":"boxWeightGrams"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}}]}}]}}]} as unknown as DocumentNode<ShippingBoxesQuery, ShippingBoxesQueryVariables>;
export const UpsertShippingBoxDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpsertShippingBox"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ShippingBoxInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"upsertShippingBox"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]} as unknown as DocumentNode<UpsertShippingBoxMutation, UpsertShippingBoxMutationVariables>;
export const DeleteShippingBoxDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteShippingBox"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteShippingBox"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}]}]}}]} as unknown as DocumentNode<DeleteShippingBoxMutation, DeleteShippingBoxMutationVariables>;