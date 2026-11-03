"use server";
import "../setup";

import * as core from "@skelet/storefront-core/account/actions";

export type * from "@skelet/storefront-core/account/actions";

export async function reorderAction(...args: Parameters<typeof core.reorderAction>) {
  return core.reorderAction(...args);
}

export async function requestReturnAction(...args: Parameters<typeof core.requestReturnAction>) {
  return core.requestReturnAction(...args);
}
