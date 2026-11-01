"use server";
import "../setup";

import * as core from "@skelet/storefront-core/webforms/actions";

export type * from "@skelet/storefront-core/webforms/actions";

export async function submitWebFormAction(...args: Parameters<typeof core.submitWebFormAction>) {
  return core.submitWebFormAction(...args);
}

export async function presignWebFormUploadAction(...args: Parameters<typeof core.presignWebFormUploadAction>) {
  return core.presignWebFormUploadAction(...args);
}
