"use server";

import { revalidateAccount } from "@/app/lib/revalidate";
import { redirect } from "next/navigation";

import { gqlFetch } from "@/app/lib/graphql";
import {
  CancelSubscriptionDocument,
  SetSubscriptionPaymentMethodDocument,
} from "@/app/lib/customer/operations.gql";

const PAGE = "/account/subscriptions";

export async function cancelSubscriptionAction(formData: FormData): Promise<void> {
  const id = String(formData.get("id") ?? "");
  if (!id) return;
  let ok = true;
  try {
    await gqlFetch(CancelSubscriptionDocument, { id });
  } catch (err) {
    // Silence here left the customer looking at a still-active subscription with no clue
     
    console.error("subscription cancel failed:", err);
    ok = false;
  }
  revalidateAccount();
  redirect(ok ? `${PAGE}?done=canceled` : `${PAGE}?error=cancel`);
}

export async function setSubscriptionPaymentMethodAction(
  formData: FormData,
): Promise<void> {
  const subscriptionId = String(formData.get("subscriptionId") ?? "");
  const paymentMethodId = String(formData.get("paymentMethodId") ?? "");
  if (!subscriptionId) return;
  let ok = true;
  try {
    await gqlFetch(SetSubscriptionPaymentMethodDocument, {
      subscriptionId,
      paymentMethodId: paymentMethodId || null,
    });
  } catch (err) {
     
    console.error("subscription card change failed:", err);
    ok = false;
  }
  revalidateAccount();
  redirect(ok ? `${PAGE}?done=card` : `${PAGE}?error=card`);
}
