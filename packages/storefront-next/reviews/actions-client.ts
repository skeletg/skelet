"use server";
import "../setup";

import * as core from "@skelet/storefront-core/reviews/actions-client";

export type * from "@skelet/storefront-core/reviews/actions-client";

export async function loadMyReviewState(...args: Parameters<typeof core.loadMyReviewState>) {
  return core.loadMyReviewState(...args);
}

export async function loadReviewsPage(...args: Parameters<typeof core.loadReviewsPage>) {
  return core.loadReviewsPage(...args);
}
