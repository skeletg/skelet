/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
import type * as Types from '../graphql/schema.types';

import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type LoyaltyStatusQueryVariables = Exact<{ [key: string]: never; }>;


export type LoyaltyStatusQuery = { loyaltyStatus: { enabled: boolean, balancePoints: number, earnPointsPerUnit: number, redeemCentsPerPoint: number } };

export type RedeemLoyaltyPointsMutationVariables = Exact<{
  points: number;
}>;


export type RedeemLoyaltyPointsMutation = { redeemLoyaltyPoints: { balancePoints: number, storeCreditBalance: number, grantedCents: number } };

export type MyReferralQueryVariables = Exact<{ [key: string]: never; }>;


export type MyReferralQuery = { myReferral: { enabled: boolean, code: string, referrerRewardCents: number, refereeRewardCents: number, referredCount: number, rewardedCount: number, earnedCents: number } };


export const LoyaltyStatusDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"LoyaltyStatus"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"loyaltyStatus"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"enabled"}},{"kind":"Field","name":{"kind":"Name","value":"balancePoints"}},{"kind":"Field","name":{"kind":"Name","value":"earnPointsPerUnit"}},{"kind":"Field","name":{"kind":"Name","value":"redeemCentsPerPoint"}}]}}]}}]} as unknown as DocumentNode<LoyaltyStatusQuery, LoyaltyStatusQueryVariables>;
export const RedeemLoyaltyPointsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RedeemLoyaltyPoints"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"points"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"redeemLoyaltyPoints"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"points"},"value":{"kind":"Variable","name":{"kind":"Name","value":"points"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"balancePoints"}},{"kind":"Field","name":{"kind":"Name","value":"storeCreditBalance"}},{"kind":"Field","name":{"kind":"Name","value":"grantedCents"}}]}}]}}]} as unknown as DocumentNode<RedeemLoyaltyPointsMutation, RedeemLoyaltyPointsMutationVariables>;
export const MyReferralDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"MyReferral"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"myReferral"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"enabled"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"referrerRewardCents"}},{"kind":"Field","name":{"kind":"Name","value":"refereeRewardCents"}},{"kind":"Field","name":{"kind":"Name","value":"referredCount"}},{"kind":"Field","name":{"kind":"Name","value":"rewardedCount"}},{"kind":"Field","name":{"kind":"Name","value":"earnedCents"}}]}}]}}]} as unknown as DocumentNode<MyReferralQuery, MyReferralQueryVariables>;