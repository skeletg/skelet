// Server-side review queries

import { REVIEW_VOTER_COOKIE } from "./voter-cookie";
import { perRequest, request } from "../request";
import { gqlFetch } from "../graphql";
import {
  ProductWithReviewsDocument,
  type ProductWithReviewsQuery,
  type ReviewSort,
} from "./operations.gql";

export type ProductReviews = NonNullable<ProductWithReviewsQuery["product"]>;
export type ReviewListItem = ProductReviews["reviews"][number];
export type ReviewQuestionWire = ProductReviews["reviewQuestions"][number];
export type ReviewAggregatesWire = ProductReviews["reviewAggregates"];
export type ReviewEligibilityWire = ProductReviews["customerCanReview"];

export const getProductWithReviews = perRequest(
  async (
    id: string,
    sort: ReviewSort = "NEWEST",
  ): Promise<{ product: ProductReviews | null }> => {
    try {
      // Forward the anonymous voter cookie so the backend can populate Review.myVote for not-signed-in browsers -
      // without it the storefront would only ever see myVote=0 for guests even when their vote is persisted server-side
      const voterToken = (await request().cookies()).get(REVIEW_VOTER_COOKIE)?.value;
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
