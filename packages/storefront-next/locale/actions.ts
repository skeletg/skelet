"use server";
import "../setup";

import * as core from "@skelet/storefront-core/locale/actions";

export async function setLocale(...args: Parameters<typeof core.setLocale>) {
  return core.setLocale(...args);
}
