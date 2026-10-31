/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
import type * as Types from '../../graphql/schema.types';

import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type ConsentChannel =
  | 'EMAIL'
  | 'PUSH'
  | 'SMS';

export type ConsentPurpose =
  | 'MARKETING'
  | 'TRANSACTIONAL';

export type SubscriptionInterval =
  | 'DAY'
  | 'MONTH'
  | 'WEEK'
  | 'YEAR';

export type SubscriptionStatus =
  | 'ACTIVE'
  | 'CANCELED'
  | 'EXPIRED'
  | 'PAUSED';

export type ConsentDefinitionsQueryVariables = Exact<{ [key: string]: never; }>;


export type ConsentDefinitionsQuery = { consentDefinitions: Array<{ slug: string, name: string, description: string, purpose: Types.ConsentPurpose, channel: Types.ConsentChannel, required: boolean, defaultOptIn: boolean }> };

export type CustomerAddressFieldsFragment = { id: string, firstName: string, lastName: string, company: string | null, street1: string, street2: string | null, city: string, state: string | null, postalCode: string, countryCode: string, phone: string | null };

export type MyProfileQueryVariables = Exact<{ [key: string]: never; }>;


export type MyProfileQuery = { myProfile: { id: string, email: string, firstName: string, lastName: string, phone: string | null, isEmailVerified: boolean, storeCreditBalance: number, defaultBillingAddress: { id: string, firstName: string, lastName: string, company: string | null, street1: string, street2: string | null, city: string, state: string | null, postalCode: string, countryCode: string, phone: string | null } | null, defaultShippingAddress: { id: string, firstName: string, lastName: string, company: string | null, street1: string, street2: string | null, city: string, state: string | null, postalCode: string, countryCode: string, phone: string | null } | null, addresses: Array<{ id: string, firstName: string, lastName: string, company: string | null, street1: string, street2: string | null, city: string, state: string | null, postalCode: string, countryCode: string, phone: string | null }> } };

export type MyDataExportQueryVariables = Exact<{ [key: string]: never; }>;


export type MyDataExportQuery = { myDataExport: string };

export type MyDownloadsQueryVariables = Exact<{ [key: string]: never; }>;


export type MyDownloadsQuery = { myDownloads: Array<{ id: string, title: string, productId: string, downloadUrl: string, downloadsUsed: number, maxDownloads: number | null, expiresAt: string | null, createdAt: string }> };

export type MySubscriptionsQueryVariables = Exact<{ [key: string]: never; }>;


export type MySubscriptionsQuery = { mySubscriptions: Array<{ id: string, productId: string, title: string, status: Types.SubscriptionStatus, intervalUnit: Types.SubscriptionInterval, intervalCount: number, price: number, currencyCode: string, currentPeriodEnd: string, nextBillingAt: string, canceledAt: string | null, createdAt: string, paymentMethodId: string | null }> };

export type CancelSubscriptionMutationVariables = Exact<{
  id: string | number;
}>;


export type CancelSubscriptionMutation = { cancelSubscription: { id: string, status: Types.SubscriptionStatus, canceledAt: string | null } };

export type SetSubscriptionPaymentMethodMutationVariables = Exact<{
  subscriptionId: string | number;
  paymentMethodId?: string | number | null | undefined;
}>;


export type SetSubscriptionPaymentMethodMutation = { setMySubscriptionPaymentMethod: boolean };

export const CustomerAddressFieldsFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"CustomerAddressFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Address"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"firstName"}},{"kind":"Field","name":{"kind":"Name","value":"lastName"}},{"kind":"Field","name":{"kind":"Name","value":"company"}},{"kind":"Field","name":{"kind":"Name","value":"street1"}},{"kind":"Field","name":{"kind":"Name","value":"street2"}},{"kind":"Field","name":{"kind":"Name","value":"city"}},{"kind":"Field","name":{"kind":"Name","value":"state"}},{"kind":"Field","name":{"kind":"Name","value":"postalCode"}},{"kind":"Field","name":{"kind":"Name","value":"countryCode"}},{"kind":"Field","name":{"kind":"Name","value":"phone"}}]}}]} as unknown as DocumentNode<CustomerAddressFieldsFragment, unknown>;
export const ConsentDefinitionsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ConsentDefinitions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"consentDefinitions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"slug"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"purpose"}},{"kind":"Field","name":{"kind":"Name","value":"channel"}},{"kind":"Field","name":{"kind":"Name","value":"required"}},{"kind":"Field","name":{"kind":"Name","value":"defaultOptIn"}}]}}]}}]} as unknown as DocumentNode<ConsentDefinitionsQuery, ConsentDefinitionsQueryVariables>;
export const MyProfileDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"MyProfile"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"myProfile"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"email"}},{"kind":"Field","name":{"kind":"Name","value":"firstName"}},{"kind":"Field","name":{"kind":"Name","value":"lastName"}},{"kind":"Field","name":{"kind":"Name","value":"phone"}},{"kind":"Field","name":{"kind":"Name","value":"isEmailVerified"}},{"kind":"Field","name":{"kind":"Name","value":"storeCreditBalance"}},{"kind":"Field","name":{"kind":"Name","value":"defaultBillingAddress"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"CustomerAddressFields"}}]}},{"kind":"Field","name":{"kind":"Name","value":"defaultShippingAddress"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"CustomerAddressFields"}}]}},{"kind":"Field","name":{"kind":"Name","value":"addresses"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"CustomerAddressFields"}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"CustomerAddressFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Address"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"firstName"}},{"kind":"Field","name":{"kind":"Name","value":"lastName"}},{"kind":"Field","name":{"kind":"Name","value":"company"}},{"kind":"Field","name":{"kind":"Name","value":"street1"}},{"kind":"Field","name":{"kind":"Name","value":"street2"}},{"kind":"Field","name":{"kind":"Name","value":"city"}},{"kind":"Field","name":{"kind":"Name","value":"state"}},{"kind":"Field","name":{"kind":"Name","value":"postalCode"}},{"kind":"Field","name":{"kind":"Name","value":"countryCode"}},{"kind":"Field","name":{"kind":"Name","value":"phone"}}]}}]} as unknown as DocumentNode<MyProfileQuery, MyProfileQueryVariables>;
export const MyDataExportDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"MyDataExport"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"myDataExport"}}]}}]} as unknown as DocumentNode<MyDataExportQuery, MyDataExportQueryVariables>;
export const MyDownloadsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"MyDownloads"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"myDownloads"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"productId"}},{"kind":"Field","name":{"kind":"Name","value":"downloadUrl"}},{"kind":"Field","name":{"kind":"Name","value":"downloadsUsed"}},{"kind":"Field","name":{"kind":"Name","value":"maxDownloads"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}}]}}]}}]} as unknown as DocumentNode<MyDownloadsQuery, MyDownloadsQueryVariables>;
export const MySubscriptionsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"MySubscriptions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"mySubscriptions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"productId"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"intervalUnit"}},{"kind":"Field","name":{"kind":"Name","value":"intervalCount"}},{"kind":"Field","name":{"kind":"Name","value":"price"}},{"kind":"Field","name":{"kind":"Name","value":"currencyCode"}},{"kind":"Field","name":{"kind":"Name","value":"currentPeriodEnd"}},{"kind":"Field","name":{"kind":"Name","value":"nextBillingAt"}},{"kind":"Field","name":{"kind":"Name","value":"canceledAt"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"paymentMethodId"}}]}}]}}]} as unknown as DocumentNode<MySubscriptionsQuery, MySubscriptionsQueryVariables>;
export const CancelSubscriptionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CancelSubscription"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"cancelSubscription"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"canceledAt"}}]}}]}}]} as unknown as DocumentNode<CancelSubscriptionMutation, CancelSubscriptionMutationVariables>;
export const SetSubscriptionPaymentMethodDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SetSubscriptionPaymentMethod"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"subscriptionId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"paymentMethodId"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"setMySubscriptionPaymentMethod"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"subscriptionId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"subscriptionId"}}},{"kind":"Argument","name":{"kind":"Name","value":"paymentMethodId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"paymentMethodId"}}}]}]}}]} as unknown as DocumentNode<SetSubscriptionPaymentMethodMutation, SetSubscriptionPaymentMethodMutationVariables>;