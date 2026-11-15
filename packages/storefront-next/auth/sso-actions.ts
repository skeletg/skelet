"use server";
import "../setup";

import * as core from "@skelet/storefront-core/auth/sso-actions";

export async function ssoStartAction(...args: Parameters<typeof core.ssoStartAction>) {
  return core.ssoStartAction(...args);
}
