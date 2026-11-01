"use server";
import "../setup";

import * as core from "@skelet/storefront-core/consent/actions";

export async function setConsentAction(...args: Parameters<typeof core.setConsentAction>) {
  return core.setConsentAction(...args);
}

export async function acceptAllConsentAction(...args: Parameters<typeof core.acceptAllConsentAction>) {
  return core.acceptAllConsentAction(...args);
}

export async function essentialOnlyConsentAction(...args: Parameters<typeof core.essentialOnlyConsentAction>) {
  return core.essentialOnlyConsentAction(...args);
}
