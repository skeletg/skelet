"use server";
import "../setup";

import * as core from "@skelet/storefront-core/payment-methods/actions";

export type * from "@skelet/storefront-core/payment-methods/actions";

export async function getMyPaymentMethods(...args: Parameters<typeof core.getMyPaymentMethods>) {
  return core.getMyPaymentMethods(...args);
}

export async function deletePaymentMethodAction(...args: Parameters<typeof core.deletePaymentMethodAction>) {
  return core.deletePaymentMethodAction(...args);
}
