"use server";
import "../setup";

import * as core from "@skelet/storefront-core/order/actions";

export async function trackOrderAction(...args: Parameters<typeof core.trackOrderAction>) {
  return core.trackOrderAction(...args);
}
