"use server";

import { revalidateAccount } from "@/app/lib/revalidate";
import { redirect } from "next/navigation";

import { gqlFetch } from "@/app/lib/graphql";
import {
  LoyaltyStatusDocument,
  MyReferralDocument,
  RedeemLoyaltyPointsDocument,
} from "@/app/lib/loyalty/operations.gql";

export type LoyaltyStatus = {
  enabled: boolean;
  balancePoints: number;
  earnPointsPerUnit: number;
  redeemCentsPerPoint: number;
};

export async function getLoyaltyStatus(): Promise<LoyaltyStatus | null> {
  try {
    const res = await gqlFetch(LoyaltyStatusDocument, {});
    return res.data.loyaltyStatus;
  } catch {
    return null;
  }
}

export async function redeemLoyaltyPointsAction(
  formData: FormData,
): Promise<void> {
  const points = Number.parseInt(String(formData.get("points") ?? ""), 10);
  if (Number.isNaN(points) || points <= 0) {
    redirect("/account?error=redeem");
  }
  let ok = true;
  try {
    await gqlFetch(RedeemLoyaltyPointsDocument, { points });
  } catch (err) {
    // Points are money - a swallowed failure reads as "my balance vanished"
     
    console.error("loyalty redeem failed:", err);
    ok = false;
  }
  revalidateAccount();
  redirect(ok ? "/account?done=redeem" : "/account?error=redeem");
}

export type ReferralInfo = {
  enabled: boolean;
  code: string;
  referrerRewardCents: number;
  refereeRewardCents: number;
  referredCount: number;
  rewardedCount: number;
  earnedCents: number;
};

export async function getMyReferral(): Promise<ReferralInfo | null> {
  try {
    const res = await gqlFetch(MyReferralDocument, {});
    return res.data.myReferral;
  } catch {
    return null;
  }
}
