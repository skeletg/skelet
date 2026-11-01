"use server";
import "../setup";

import * as core from "@skelet/storefront-core/beacons/actions";

export type * from "@skelet/storefront-core/beacons/actions";

export async function notifyBackInStockAction(...args: Parameters<typeof core.notifyBackInStockAction>) {
  return core.notifyBackInStockAction(...args);
}

export async function submitSignupAction(...args: Parameters<typeof core.submitSignupAction>) {
  return core.submitSignupAction(...args);
}
