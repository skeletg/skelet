"use server";
import "../setup";

import * as core from "@skelet/storefront-core/loyalty/actions";

export type * from "@skelet/storefront-core/loyalty/actions";

export async function getLoyaltyStatus(...args: Parameters<typeof core.getLoyaltyStatus>) {
  return core.getLoyaltyStatus(...args);
}

export async function redeemLoyaltyPointsAction(...args: Parameters<typeof core.redeemLoyaltyPointsAction>) {
  return core.redeemLoyaltyPointsAction(...args);
}

export async function getMyReferral(...args: Parameters<typeof core.getMyReferral>) {
  return core.getMyReferral(...args);
}
