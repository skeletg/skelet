"use server";
import "../setup";

import * as core from "@skelet/storefront-core/customer/actions-client";

export type * from "@skelet/storefront-core/customer/actions-client";

export async function loadHeaderIdentity(...args: Parameters<typeof core.loadHeaderIdentity>) {
  return core.loadHeaderIdentity(...args);
}
