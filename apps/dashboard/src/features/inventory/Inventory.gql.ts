/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
import type * as Types from '../../graphql/schema.types';

import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type AdjustInventoryLevelInput = {
  locationId: string | number;
  newQuantity: number;
  note?: string | null | undefined;
  productId: string | number;
};

export type CreateInventoryLocationInput = {
  city?: string | null | undefined;
  contactName?: string | null | undefined;
  countryCode?: string | null | undefined;
  email?: string | null | undefined;
  externalReference?: string | null | undefined;
  hours?: unknown;
  isActive?: boolean | null | undefined;
  latitude?: number | null | undefined;
  line1?: string | null | undefined;
  line2?: string | null | undefined;
  longitude?: number | null | undefined;
  metadata?: Array<MetadataInput> | null | undefined;
  name: string;
  phone?: string | null | undefined;
  postalCode?: string | null | undefined;
  region?: string | null | undefined;
  slug: string;
  timezone?: string | null | undefined;
  type?: string | null | undefined;
};

export type InventoryMode =
  | 'FLAT'
  | 'MULTI_LOCATION';

export type LocationAssignmentInput = {
  locationId: string | number;
  priority: number;
};

export type MetadataInput = {
  key: string;
  value: string;
};

export type SyncProductInventoryInput = {
  productId?: string | number | null | undefined;
  productSku?: string | null | undefined;
  quantity: number;
  source?: string | null | undefined;
};

export type UpdateInventoryLocationInput = {
  city?: string | null | undefined;
  clearCoords?: boolean | null | undefined;
  contactName?: string | null | undefined;
  countryCode?: string | null | undefined;
  email?: string | null | undefined;
  externalReference?: string | null | undefined;
  hours?: unknown;
  id: string | number;
  isActive?: boolean | null | undefined;
  latitude?: number | null | undefined;
  line1?: string | null | undefined;
  line2?: string | null | undefined;
  longitude?: number | null | undefined;
  metadata?: Array<MetadataInput> | null | undefined;
  name?: string | null | undefined;
  phone?: string | null | undefined;
  postalCode?: string | null | undefined;
  region?: string | null | undefined;
  timezone?: string | null | undefined;
  type?: string | null | undefined;
};

export type InventoryLocationsPagedQueryVariables = Exact<{
  search?: string | null | undefined;
  limit?: number | null | undefined;
  offset?: number | null | undefined;
}>;


export type InventoryLocationsPagedQuery = { inventoryLocationsCount: number, inventoryLocations: Array<{ id: string, slug: string, name: string, isActive: boolean, type: string, city: string, region: string, countryCode: string, pickupEnabled: boolean }> };

export type InventoryLocationsQueryVariables = Exact<{ [key: string]: never; }>;


export type InventoryLocationsQuery = { inventoryLocations: Array<{ id: string, slug: string, name: string, isActive: boolean, type: string, line1: string, line2: string, city: string, region: string, postalCode: string, countryCode: string, latitude: number | null, longitude: number | null, timezone: string, hours: unknown, email: string, phone: string, contactName: string, externalReference: string, pickupEnabled: boolean, pickupInstructions: string, createdAt: string, updatedAt: string, metadata: Array<{ key: string, value: string }> }> };

export type InventoryLocationQueryVariables = Exact<{
  id: string | number;
}>;


export type InventoryLocationQuery = { inventoryLocation: { id: string, slug: string, name: string, isActive: boolean, type: string, line1: string, line2: string, city: string, region: string, postalCode: string, countryCode: string, latitude: number | null, longitude: number | null, timezone: string, hours: unknown, email: string, phone: string, contactName: string, externalReference: string, pickupEnabled: boolean, pickupInstructions: string, createdAt: string, updatedAt: string, metadata: Array<{ key: string, value: string }> } };

export type ProductInventoryQueryVariables = Exact<{
  productId: string | number;
}>;


export type ProductInventoryQuery = { productInventory: { id: string, tracked: boolean, backorderable: boolean, lowStockThreshold: number, quantity: number, lastSyncedAt: string | null, lastSyncedSource: string | null, levels: Array<{ quantity: number, reservedQuantity: number, available: number, location: { id: string, slug: string, name: string, isActive: boolean } }>, product: { id: string, url: string, quantityDecimals: number, quantityStep: number } } | null };

export type InventoryModeQueryVariables = Exact<{ [key: string]: never; }>;


export type InventoryModeQuery = { inventoryMode: Types.InventoryMode };

export type LowStockLevelsQueryVariables = Exact<{
  limit?: number | null | undefined;
}>;


export type LowStockLevelsQuery = { lowStockLevels: Array<{ inventoryItemId: string, productId: string, productSlug: string, productSku: string, locationId: string | null, locationSlug: string | null, locationName: string | null, quantity: number, lowStockThreshold: number, backorderable: boolean, quantityDecimals: number, quantityStep: number, updatedAt: string }> };

export type ChannelWithLocationsQueryVariables = Exact<{
  id: string | number;
}>;


export type ChannelWithLocationsQuery = { channel: { id: string, slug: string, name: string, inventoryLocations: Array<{ priority: number, location: { id: string, slug: string, name: string, isActive: boolean } }> } };

export type CreateInventoryLocationMutationVariables = Exact<{
  input: Types.CreateInventoryLocationInput;
}>;


export type CreateInventoryLocationMutation = { createInventoryLocation: { id: string, slug: string, name: string, isActive: boolean, type: string, line1: string, line2: string, city: string, region: string, postalCode: string, countryCode: string, latitude: number | null, longitude: number | null, timezone: string, hours: unknown, email: string, phone: string, contactName: string, externalReference: string, createdAt: string, updatedAt: string, metadata: Array<{ key: string, value: string }> } };

export type UpdateInventoryLocationMutationVariables = Exact<{
  input: Types.UpdateInventoryLocationInput;
}>;


export type UpdateInventoryLocationMutation = { updateInventoryLocation: { id: string, slug: string, name: string, isActive: boolean, type: string, line1: string, line2: string, city: string, region: string, postalCode: string, countryCode: string, latitude: number | null, longitude: number | null, timezone: string, hours: unknown, email: string, phone: string, contactName: string, externalReference: string, updatedAt: string, metadata: Array<{ key: string, value: string }> } };

export type DeleteInventoryLocationMutationVariables = Exact<{
  id: string | number;
}>;


export type DeleteInventoryLocationMutation = { deleteInventoryLocation: boolean };

export type SetChannelLocationsMutationVariables = Exact<{
  channelId: string | number;
  assignments: Array<Types.LocationAssignmentInput> | Types.LocationAssignmentInput;
}>;


export type SetChannelLocationsMutation = { setChannelLocations: { id: string, inventoryLocations: Array<{ priority: number, location: { id: string, slug: string, name: string, isActive: boolean } }> } };

export type SetInventoryItemTrackingMutationVariables = Exact<{
  productId: string | number;
  tracked: boolean;
}>;


export type SetInventoryItemTrackingMutation = { setInventoryItemTracking: { id: string, tracked: boolean, backorderable: boolean, lowStockThreshold: number } };

export type SetInventoryItemBackorderMutationVariables = Exact<{
  productId: string | number;
  backorderable: boolean;
  lowStockThreshold?: number | null | undefined;
}>;


export type SetInventoryItemBackorderMutation = { setInventoryItemBackorder: { id: string, tracked: boolean, backorderable: boolean, lowStockThreshold: number } };

export type AdjustInventoryLevelMutationVariables = Exact<{
  input: Types.AdjustInventoryLevelInput;
}>;


export type AdjustInventoryLevelMutation = { adjustInventoryLevel: { quantity: number, reservedQuantity: number, available: number, location: { id: string, slug: string, name: string } } };

export type SyncProductInventoryMutationVariables = Exact<{
  input: Types.SyncProductInventoryInput;
}>;


export type SyncProductInventoryMutation = { syncProductInventory: { id: string, tracked: boolean, backorderable: boolean, lowStockThreshold: number, quantity: number, lastSyncedAt: string | null, lastSyncedSource: string | null } };

export type SyncProductInventoryBatchMutationVariables = Exact<{
  items: Array<Types.SyncProductInventoryInput> | Types.SyncProductInventoryInput;
}>;


export type SyncProductInventoryBatchMutation = { syncProductInventoryBatch: { succeeded: number, failed: Array<{ productSku: string | null, productId: string | null, reason: string }> } };

export type SetLocationPickupMutationVariables = Exact<{
  id: string | number;
  enabled: boolean;
  instructions?: string | null | undefined;
}>;


export type SetLocationPickupMutation = { setLocationPickup: { id: string, pickupEnabled: boolean, pickupInstructions: string } };


export const InventoryLocationsPagedDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"InventoryLocationsPaged"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"search"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"offset"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"inventoryLocations"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"search"},"value":{"kind":"Variable","name":{"kind":"Name","value":"search"}}},{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}},{"kind":"Argument","name":{"kind":"Name","value":"offset"},"value":{"kind":"Variable","name":{"kind":"Name","value":"offset"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"city"}},{"kind":"Field","name":{"kind":"Name","value":"region"}},{"kind":"Field","name":{"kind":"Name","value":"countryCode"}},{"kind":"Field","name":{"kind":"Name","value":"pickupEnabled"}}]}},{"kind":"Field","name":{"kind":"Name","value":"inventoryLocationsCount"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"search"},"value":{"kind":"Variable","name":{"kind":"Name","value":"search"}}}]}]}}]} as unknown as DocumentNode<InventoryLocationsPagedQuery, InventoryLocationsPagedQueryVariables>;
export const InventoryLocationsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"InventoryLocations"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"inventoryLocations"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"line1"}},{"kind":"Field","name":{"kind":"Name","value":"line2"}},{"kind":"Field","name":{"kind":"Name","value":"city"}},{"kind":"Field","name":{"kind":"Name","value":"region"}},{"kind":"Field","name":{"kind":"Name","value":"postalCode"}},{"kind":"Field","name":{"kind":"Name","value":"countryCode"}},{"kind":"Field","name":{"kind":"Name","value":"latitude"}},{"kind":"Field","name":{"kind":"Name","value":"longitude"}},{"kind":"Field","name":{"kind":"Name","value":"timezone"}},{"kind":"Field","name":{"kind":"Name","value":"hours"}},{"kind":"Field","name":{"kind":"Name","value":"email"}},{"kind":"Field","name":{"kind":"Name","value":"phone"}},{"kind":"Field","name":{"kind":"Name","value":"contactName"}},{"kind":"Field","name":{"kind":"Name","value":"externalReference"}},{"kind":"Field","name":{"kind":"Name","value":"pickupEnabled"}},{"kind":"Field","name":{"kind":"Name","value":"pickupInstructions"}},{"kind":"Field","name":{"kind":"Name","value":"metadata"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"key"}},{"kind":"Field","name":{"kind":"Name","value":"value"}}]}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]}}]} as unknown as DocumentNode<InventoryLocationsQuery, InventoryLocationsQueryVariables>;
export const InventoryLocationDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"InventoryLocation"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"inventoryLocation"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"line1"}},{"kind":"Field","name":{"kind":"Name","value":"line2"}},{"kind":"Field","name":{"kind":"Name","value":"city"}},{"kind":"Field","name":{"kind":"Name","value":"region"}},{"kind":"Field","name":{"kind":"Name","value":"postalCode"}},{"kind":"Field","name":{"kind":"Name","value":"countryCode"}},{"kind":"Field","name":{"kind":"Name","value":"latitude"}},{"kind":"Field","name":{"kind":"Name","value":"longitude"}},{"kind":"Field","name":{"kind":"Name","value":"timezone"}},{"kind":"Field","name":{"kind":"Name","value":"hours"}},{"kind":"Field","name":{"kind":"Name","value":"email"}},{"kind":"Field","name":{"kind":"Name","value":"phone"}},{"kind":"Field","name":{"kind":"Name","value":"contactName"}},{"kind":"Field","name":{"kind":"Name","value":"externalReference"}},{"kind":"Field","name":{"kind":"Name","value":"pickupEnabled"}},{"kind":"Field","name":{"kind":"Name","value":"pickupInstructions"}},{"kind":"Field","name":{"kind":"Name","value":"metadata"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"key"}},{"kind":"Field","name":{"kind":"Name","value":"value"}}]}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]}}]} as unknown as DocumentNode<InventoryLocationQuery, InventoryLocationQueryVariables>;
export const ProductInventoryDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ProductInventory"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"productId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"productInventory"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"productId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"productId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"tracked"}},{"kind":"Field","name":{"kind":"Name","value":"backorderable"}},{"kind":"Field","name":{"kind":"Name","value":"lowStockThreshold"}},{"kind":"Field","name":{"kind":"Name","value":"quantity"}},{"kind":"Field","name":{"kind":"Name","value":"lastSyncedAt"}},{"kind":"Field","name":{"kind":"Name","value":"lastSyncedSource"}},{"kind":"Field","name":{"kind":"Name","value":"levels"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"location"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}}]}},{"kind":"Field","name":{"kind":"Name","value":"quantity"}},{"kind":"Field","name":{"kind":"Name","value":"reservedQuantity"}},{"kind":"Field","name":{"kind":"Name","value":"available"}}]}},{"kind":"Field","name":{"kind":"Name","value":"product"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"quantityDecimals"}},{"kind":"Field","name":{"kind":"Name","value":"quantityStep"}}]}}]}}]}}]} as unknown as DocumentNode<ProductInventoryQuery, ProductInventoryQueryVariables>;
export const InventoryModeDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"InventoryMode"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"inventoryMode"}}]}}]} as unknown as DocumentNode<InventoryModeQuery, InventoryModeQueryVariables>;
export const LowStockLevelsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"LowStockLevels"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"lowStockLevels"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"inventoryItemId"}},{"kind":"Field","name":{"kind":"Name","value":"productId"}},{"kind":"Field","name":{"kind":"Name","value":"productSlug"}},{"kind":"Field","name":{"kind":"Name","value":"productSku"}},{"kind":"Field","name":{"kind":"Name","value":"locationId"}},{"kind":"Field","name":{"kind":"Name","value":"locationSlug"}},{"kind":"Field","name":{"kind":"Name","value":"locationName"}},{"kind":"Field","name":{"kind":"Name","value":"quantity"}},{"kind":"Field","name":{"kind":"Name","value":"lowStockThreshold"}},{"kind":"Field","name":{"kind":"Name","value":"backorderable"}},{"kind":"Field","name":{"kind":"Name","value":"quantityDecimals"}},{"kind":"Field","name":{"kind":"Name","value":"quantityStep"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]}}]} as unknown as DocumentNode<LowStockLevelsQuery, LowStockLevelsQueryVariables>;
export const ChannelWithLocationsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ChannelWithLocations"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"channel"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"inventoryLocations"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"priority"}},{"kind":"Field","name":{"kind":"Name","value":"location"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}}]}}]}}]}}]}}]} as unknown as DocumentNode<ChannelWithLocationsQuery, ChannelWithLocationsQueryVariables>;
export const CreateInventoryLocationDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateInventoryLocation"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"CreateInventoryLocationInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createInventoryLocation"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"line1"}},{"kind":"Field","name":{"kind":"Name","value":"line2"}},{"kind":"Field","name":{"kind":"Name","value":"city"}},{"kind":"Field","name":{"kind":"Name","value":"region"}},{"kind":"Field","name":{"kind":"Name","value":"postalCode"}},{"kind":"Field","name":{"kind":"Name","value":"countryCode"}},{"kind":"Field","name":{"kind":"Name","value":"latitude"}},{"kind":"Field","name":{"kind":"Name","value":"longitude"}},{"kind":"Field","name":{"kind":"Name","value":"timezone"}},{"kind":"Field","name":{"kind":"Name","value":"hours"}},{"kind":"Field","name":{"kind":"Name","value":"email"}},{"kind":"Field","name":{"kind":"Name","value":"phone"}},{"kind":"Field","name":{"kind":"Name","value":"contactName"}},{"kind":"Field","name":{"kind":"Name","value":"externalReference"}},{"kind":"Field","name":{"kind":"Name","value":"metadata"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"key"}},{"kind":"Field","name":{"kind":"Name","value":"value"}}]}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]}}]} as unknown as DocumentNode<CreateInventoryLocationMutation, CreateInventoryLocationMutationVariables>;
export const UpdateInventoryLocationDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateInventoryLocation"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateInventoryLocationInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateInventoryLocation"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}},{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"line1"}},{"kind":"Field","name":{"kind":"Name","value":"line2"}},{"kind":"Field","name":{"kind":"Name","value":"city"}},{"kind":"Field","name":{"kind":"Name","value":"region"}},{"kind":"Field","name":{"kind":"Name","value":"postalCode"}},{"kind":"Field","name":{"kind":"Name","value":"countryCode"}},{"kind":"Field","name":{"kind":"Name","value":"latitude"}},{"kind":"Field","name":{"kind":"Name","value":"longitude"}},{"kind":"Field","name":{"kind":"Name","value":"timezone"}},{"kind":"Field","name":{"kind":"Name","value":"hours"}},{"kind":"Field","name":{"kind":"Name","value":"email"}},{"kind":"Field","name":{"kind":"Name","value":"phone"}},{"kind":"Field","name":{"kind":"Name","value":"contactName"}},{"kind":"Field","name":{"kind":"Name","value":"externalReference"}},{"kind":"Field","name":{"kind":"Name","value":"metadata"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"key"}},{"kind":"Field","name":{"kind":"Name","value":"value"}}]}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]}}]} as unknown as DocumentNode<UpdateInventoryLocationMutation, UpdateInventoryLocationMutationVariables>;
export const DeleteInventoryLocationDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteInventoryLocation"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteInventoryLocation"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}]}]}}]} as unknown as DocumentNode<DeleteInventoryLocationMutation, DeleteInventoryLocationMutationVariables>;
export const SetChannelLocationsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SetChannelLocations"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"assignments"}},"type":{"kind":"NonNullType","type":{"kind":"ListType","type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"LocationAssignmentInput"}}}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"setChannelLocations"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"channelId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}}},{"kind":"Argument","name":{"kind":"Name","value":"assignments"},"value":{"kind":"Variable","name":{"kind":"Name","value":"assignments"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"inventoryLocations"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"priority"}},{"kind":"Field","name":{"kind":"Name","value":"location"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}}]}}]}}]}}]}}]} as unknown as DocumentNode<SetChannelLocationsMutation, SetChannelLocationsMutationVariables>;
export const SetInventoryItemTrackingDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SetInventoryItemTracking"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"productId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"tracked"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Boolean"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"setInventoryItemTracking"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"productId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"productId"}}},{"kind":"Argument","name":{"kind":"Name","value":"tracked"},"value":{"kind":"Variable","name":{"kind":"Name","value":"tracked"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"tracked"}},{"kind":"Field","name":{"kind":"Name","value":"backorderable"}},{"kind":"Field","name":{"kind":"Name","value":"lowStockThreshold"}}]}}]}}]} as unknown as DocumentNode<SetInventoryItemTrackingMutation, SetInventoryItemTrackingMutationVariables>;
export const SetInventoryItemBackorderDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SetInventoryItemBackorder"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"productId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"backorderable"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Boolean"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"lowStockThreshold"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Float"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"setInventoryItemBackorder"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"productId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"productId"}}},{"kind":"Argument","name":{"kind":"Name","value":"backorderable"},"value":{"kind":"Variable","name":{"kind":"Name","value":"backorderable"}}},{"kind":"Argument","name":{"kind":"Name","value":"lowStockThreshold"},"value":{"kind":"Variable","name":{"kind":"Name","value":"lowStockThreshold"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"tracked"}},{"kind":"Field","name":{"kind":"Name","value":"backorderable"}},{"kind":"Field","name":{"kind":"Name","value":"lowStockThreshold"}}]}}]}}]} as unknown as DocumentNode<SetInventoryItemBackorderMutation, SetInventoryItemBackorderMutationVariables>;
export const AdjustInventoryLevelDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"AdjustInventoryLevel"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"AdjustInventoryLevelInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"adjustInventoryLevel"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"location"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}},{"kind":"Field","name":{"kind":"Name","value":"quantity"}},{"kind":"Field","name":{"kind":"Name","value":"reservedQuantity"}},{"kind":"Field","name":{"kind":"Name","value":"available"}}]}}]}}]} as unknown as DocumentNode<AdjustInventoryLevelMutation, AdjustInventoryLevelMutationVariables>;
export const SyncProductInventoryDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SyncProductInventory"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"SyncProductInventoryInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"syncProductInventory"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"tracked"}},{"kind":"Field","name":{"kind":"Name","value":"backorderable"}},{"kind":"Field","name":{"kind":"Name","value":"lowStockThreshold"}},{"kind":"Field","name":{"kind":"Name","value":"quantity"}},{"kind":"Field","name":{"kind":"Name","value":"lastSyncedAt"}},{"kind":"Field","name":{"kind":"Name","value":"lastSyncedSource"}}]}}]}}]} as unknown as DocumentNode<SyncProductInventoryMutation, SyncProductInventoryMutationVariables>;
export const SyncProductInventoryBatchDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SyncProductInventoryBatch"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"items"}},"type":{"kind":"NonNullType","type":{"kind":"ListType","type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"SyncProductInventoryInput"}}}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"syncProductInventoryBatch"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"items"},"value":{"kind":"Variable","name":{"kind":"Name","value":"items"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"succeeded"}},{"kind":"Field","name":{"kind":"Name","value":"failed"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"productSku"}},{"kind":"Field","name":{"kind":"Name","value":"productId"}},{"kind":"Field","name":{"kind":"Name","value":"reason"}}]}}]}}]}}]} as unknown as DocumentNode<SyncProductInventoryBatchMutation, SyncProductInventoryBatchMutationVariables>;
export const SetLocationPickupDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SetLocationPickup"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"enabled"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Boolean"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"instructions"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"setLocationPickup"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}},{"kind":"Argument","name":{"kind":"Name","value":"enabled"},"value":{"kind":"Variable","name":{"kind":"Name","value":"enabled"}}},{"kind":"Argument","name":{"kind":"Name","value":"instructions"},"value":{"kind":"Variable","name":{"kind":"Name","value":"instructions"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"pickupEnabled"}},{"kind":"Field","name":{"kind":"Name","value":"pickupInstructions"}}]}}]}}]} as unknown as DocumentNode<SetLocationPickupMutation, SetLocationPickupMutationVariables>;