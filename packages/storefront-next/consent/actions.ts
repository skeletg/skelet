"use server";
import "../setup";

import * as core from "@skelet/storefront-core/consent/actions";

export async function acceptAllConsentAction(...args: Parameters<typeof core.acceptAllConsentAction>) {
  return core.acceptAllConsentAction(...args);
}

export async function essentialOnlyConsentAction(...args: Parameters<typeof core.essentialOnlyConsentAction>) {
  return core.essentialOnlyConsentAction(...args);
}

export async function saveConsentAction(...args: Parameters<typeof core.saveConsentAction>) {
  return core.saveConsentAction(...args);
}
