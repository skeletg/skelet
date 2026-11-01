"use server";
import "../setup";

import * as core from "@skelet/storefront-core/subscriptions/actions";

export async function cancelSubscriptionAction(...args: Parameters<typeof core.cancelSubscriptionAction>) {
  return core.cancelSubscriptionAction(...args);
}

export async function setSubscriptionPaymentMethodAction(...args: Parameters<typeof core.setSubscriptionPaymentMethodAction>) {
  return core.setSubscriptionPaymentMethodAction(...args);
}
