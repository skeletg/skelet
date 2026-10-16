/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
import type * as Types from '../../graphql/schema.types';

import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type BlockFloor =
  | 'CONFIGURABLE'
  | 'LOCKED'
  | 'NA';

export type BlockMode =
  | 'HIDDEN'
  | 'OPTIONAL'
  | 'REQUIRED';

export type DeliveryOptionInput = {
  blockModes?: unknown;
  carrierSlug?: string | null | undefined;
  description?: string | null | undefined;
  enabled: boolean;
  kind?: string | null | undefined;
  name: string;
  pointSource?: string | null | undefined;
  sortOrder?: number | null | undefined;
};

export type DeliveryPointInput = {
  city: string;
  countryCode: string;
  hours?: string | null | undefined;
  id?: string | number | null | undefined;
  instructions?: string | null | undefined;
  isActive: boolean;
  name: string;
  postalCode: string;
  region?: string | null | undefined;
  street1: string;
  street2?: string | null | undefined;
};

export type CheckoutSettingsQueryVariables = Exact<{
  channelId: string | number;
}>;


export type CheckoutSettingsQuery = { checkoutConfiguration: { channelId: string, usingDefaults: boolean, digitalBlocks: Array<{ type: string, label: string, floor: Types.BlockFloor, mode: Types.BlockMode, reason: string }>, kinds: Array<{ slug: string, label: string, description: string, shipTo: string, pointLabel: string | null, owner: string, blocks: Array<{ type: string, label: string, floor: Types.BlockFloor, default: Types.BlockMode | null, reason: string }> }>, options: Array<{ id: string, kind: string, kindLabel: string, name: string, description: string, enabled: boolean, sortOrder: number, blockModes: unknown, pointSource: string, carrierSlug: string, implicit: boolean, ready: boolean, readiness: Array<{ code: string, label: string, ok: boolean, detail: string, tab: string, advisory: boolean }> }>, pointSources: Array<{ slug: string, label: string, active: boolean, kinds: Array<string>, remote: boolean }>, carrierKinds: Array<{ kind: string, carriers: Array<string>, carrierRefs: Array<{ slug: string, name: string }> }>, deliveryPoints: Array<{ id: string, name: string, street1: string, street2: string, city: string, region: string, postalCode: string, countryCode: string, instructions: string, hours: string, isActive: boolean }>, readiness: Array<{ code: string, label: string, ok: boolean, detail: string, tab: string, advisory: boolean }> } };

export type CreateCheckoutOptionMutationVariables = Exact<{
  channelId: string | number;
  input: Types.DeliveryOptionInput;
}>;


export type CreateCheckoutOptionMutation = { createDeliveryOption: { id: string } };

export type UpdateCheckoutOptionMutationVariables = Exact<{
  id: string | number;
  input: Types.DeliveryOptionInput;
}>;


export type UpdateCheckoutOptionMutation = { updateDeliveryOption: { id: string } };

export type DeleteCheckoutOptionMutationVariables = Exact<{
  id: string | number;
}>;


export type DeleteCheckoutOptionMutation = { deleteDeliveryOption: boolean };

export type AdoptCheckoutOptionsMutationVariables = Exact<{
  channelId: string | number;
}>;


export type AdoptCheckoutOptionsMutation = { adoptDefaultDeliveryOptions: { channelId: string } };

export type SaveCheckoutDeliveryPointsMutationVariables = Exact<{
  channelId: string | number;
  points: Array<Types.DeliveryPointInput> | Types.DeliveryPointInput;
}>;


export type SaveCheckoutDeliveryPointsMutation = { saveDeliveryPoints: Array<{ id: string }> };


export const CheckoutSettingsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"CheckoutSettings"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"checkoutConfiguration"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"channelId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"channelId"}},{"kind":"Field","name":{"kind":"Name","value":"usingDefaults"}},{"kind":"Field","name":{"kind":"Name","value":"digitalBlocks"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"label"}},{"kind":"Field","name":{"kind":"Name","value":"floor"}},{"kind":"Field","name":{"kind":"Name","value":"mode"}},{"kind":"Field","name":{"kind":"Name","value":"reason"}}]}},{"kind":"Field","name":{"kind":"Name","value":"kinds"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"label"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"shipTo"}},{"kind":"Field","name":{"kind":"Name","value":"pointLabel"}},{"kind":"Field","name":{"kind":"Name","value":"owner"}},{"kind":"Field","name":{"kind":"Name","value":"blocks"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"label"}},{"kind":"Field","name":{"kind":"Name","value":"floor"}},{"kind":"Field","name":{"kind":"Name","value":"default"}},{"kind":"Field","name":{"kind":"Name","value":"reason"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"options"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"kind"}},{"kind":"Field","name":{"kind":"Name","value":"kindLabel"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"enabled"}},{"kind":"Field","name":{"kind":"Name","value":"sortOrder"}},{"kind":"Field","name":{"kind":"Name","value":"blockModes"}},{"kind":"Field","name":{"kind":"Name","value":"pointSource"}},{"kind":"Field","name":{"kind":"Name","value":"carrierSlug"}},{"kind":"Field","name":{"kind":"Name","value":"implicit"}},{"kind":"Field","name":{"kind":"Name","value":"ready"}},{"kind":"Field","name":{"kind":"Name","value":"readiness"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"label"}},{"kind":"Field","name":{"kind":"Name","value":"ok"}},{"kind":"Field","name":{"kind":"Name","value":"detail"}},{"kind":"Field","name":{"kind":"Name","value":"tab"}},{"kind":"Field","name":{"kind":"Name","value":"advisory"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"pointSources"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"label"}},{"kind":"Field","name":{"kind":"Name","value":"active"}},{"kind":"Field","name":{"kind":"Name","value":"kinds"}},{"kind":"Field","name":{"kind":"Name","value":"remote"}}]}},{"kind":"Field","name":{"kind":"Name","value":"carrierKinds"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"kind"}},{"kind":"Field","name":{"kind":"Name","value":"carriers"}},{"kind":"Field","name":{"kind":"Name","value":"carrierRefs"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"deliveryPoints"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"street1"}},{"kind":"Field","name":{"kind":"Name","value":"street2"}},{"kind":"Field","name":{"kind":"Name","value":"city"}},{"kind":"Field","name":{"kind":"Name","value":"region"}},{"kind":"Field","name":{"kind":"Name","value":"postalCode"}},{"kind":"Field","name":{"kind":"Name","value":"countryCode"}},{"kind":"Field","name":{"kind":"Name","value":"instructions"}},{"kind":"Field","name":{"kind":"Name","value":"hours"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}}]}},{"kind":"Field","name":{"kind":"Name","value":"readiness"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"label"}},{"kind":"Field","name":{"kind":"Name","value":"ok"}},{"kind":"Field","name":{"kind":"Name","value":"detail"}},{"kind":"Field","name":{"kind":"Name","value":"tab"}},{"kind":"Field","name":{"kind":"Name","value":"advisory"}}]}}]}}]}}]} as unknown as DocumentNode<CheckoutSettingsQuery, CheckoutSettingsQueryVariables>;
export const CreateCheckoutOptionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateCheckoutOption"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"DeliveryOptionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createDeliveryOption"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"channelId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}}},{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]} as unknown as DocumentNode<CreateCheckoutOptionMutation, CreateCheckoutOptionMutationVariables>;
export const UpdateCheckoutOptionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateCheckoutOption"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"DeliveryOptionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateDeliveryOption"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}},{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]} as unknown as DocumentNode<UpdateCheckoutOptionMutation, UpdateCheckoutOptionMutationVariables>;
export const DeleteCheckoutOptionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteCheckoutOption"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteDeliveryOption"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}]}]}}]} as unknown as DocumentNode<DeleteCheckoutOptionMutation, DeleteCheckoutOptionMutationVariables>;
export const AdoptCheckoutOptionsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"AdoptCheckoutOptions"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"adoptDefaultDeliveryOptions"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"channelId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"channelId"}}]}}]}}]} as unknown as DocumentNode<AdoptCheckoutOptionsMutation, AdoptCheckoutOptionsMutationVariables>;
export const SaveCheckoutDeliveryPointsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SaveCheckoutDeliveryPoints"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"points"}},"type":{"kind":"NonNullType","type":{"kind":"ListType","type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"DeliveryPointInput"}}}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"saveDeliveryPoints"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"channelId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}}},{"kind":"Argument","name":{"kind":"Name","value":"points"},"value":{"kind":"Variable","name":{"kind":"Name","value":"points"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]} as unknown as DocumentNode<SaveCheckoutDeliveryPointsMutation, SaveCheckoutDeliveryPointsMutationVariables>;