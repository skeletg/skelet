"use server";
import "../setup";

import * as core from "@skelet/storefront-core/reviews/actions";

export async function requestReviewMediaUploadAction(...args: Parameters<typeof core.requestReviewMediaUploadAction>) {
  return core.requestReviewMediaUploadAction(...args);
}

export async function voteReviewAction(...args: Parameters<typeof core.voteReviewAction>) {
  return core.voteReviewAction(...args);
}

export async function submitReviewAction(...args: Parameters<typeof core.submitReviewAction>) {
  return core.submitReviewAction(...args);
}
