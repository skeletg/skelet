// Server-side review queries

import { cache } from "react";

import { cookies } from "next/headers";

import { gqlFetch } from "../graphql";
import {
  ProductWithReviewsDocument,
  type ProductWithReviewsQuery,
  type ReviewSort,
} from "./operations.gql";

const REVIEW_VOTER_COOKIE = "rvid";

export type ProductReviews = NonNullable<ProductWithReviewsQuery["product"]>;
export type ReviewListItem = ProductReviews["reviews"][number];
export type ReviewQuestionWire = ProductReviews["reviewQuestions"][number];
export type ReviewAggregatesWire = ProductReviews["reviewAggregates"];
export type ReviewEligibilityWire = ProductReviews["customerCanReview"];

export const getProductWithReviews = cache(
  async (
    id: string,
    sort: ReviewSort = "NEWEST",
  ): Promise<{ product: ProductReviews | null }> => {
    try {
      // Forward the anonymous voter cookie so the backend can populate Review.myVote for not-signed-in browsers -
      // without it the storefront would only ever see myVote=0 for guests even when their vote is persisted server-side
      const voterToken = (await cookies()).get(REVIEW_VOTER_COOKIE)?.value;
      const res = await gqlFetch(
        ProductWithReviewsDocument,
        { id, sort },
        voterToken
          ? { extraHeaders: { "X-Review-Voter-Token": voterToken } }
          : undefined,
      );
      return { product: res.data.product };
    } catch {
      // Reviews are a soft dependency - a backend error shouldn't blow up the PDP.
      // Return null so the section renders an empty state
      return { product: null };
    }
  },
);
