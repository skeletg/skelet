
import { request } from "../request";
import { gqlFetch } from "../graphql";
import {
  MyReviewStateDocument,
  type MyReviewStateQuery,
  ReviewsPageDocument,
  type ReviewsPageQuery,
  type ReviewSort,
} from "./operations.gql";

const REVIEW_VOTER_COOKIE = "rvid";

export type ReviewEligibility = NonNullable<
  MyReviewStateQuery["product"]
>["customerCanReview"];

export type MyReviewState = {
  eligibility: ReviewEligibility | null;
  votes: Record<string, number>;
};

const EMPTY: MyReviewState = { eligibility: null, votes: {} };

// The PDP is cached HTML, so its server render carries no session and no voter cookie - eligibility and per-review votes would bake in as
// anonymous for everyone. This runs after hydration instead, where the request's own cookies are readable
export async function loadMyReviewState(
  productId: string,
  sort: ReviewSort = "NEWEST",
): Promise<MyReviewState> {
  try {
    // Guests vote too - without this cookie the backend can only answer myVote=0 for them
    const voterToken = (await request().cookies()).get(REVIEW_VOTER_COOKIE)?.value;
    const res = await gqlFetch(
      MyReviewStateDocument,
      { productId, sort },
      voterToken
        ? { extraHeaders: { "X-Review-Voter-Token": voterToken } }
        : undefined,
    );
    const product = res.data.product;
    if (!product) return EMPTY;
    return {
      eligibility: product.customerCanReview,
      votes: Object.fromEntries(product.reviews.map((r) => [r.id, r.myVote])),
    };
  } catch {
    // Soft dependency - the section keeps its server-rendered anonymous state
    return EMPTY;
  }
}

export type ReviewsPageItem = NonNullable<
  ReviewsPageQuery["product"]
>["reviews"][number];

// Re-sorted review page for the client-side sort buttons - the cached PDP HTML only ever
// carries the NEWEST page, so other orders are fetched here instead of via URL params.
// Voter cookie forwarded for the same reason as loadMyReviewState
export async function loadReviewsPage(
  productId: string,
  sort: ReviewSort,
): Promise<ReviewsPageItem[] | null> {
  try {
    const voterToken = (await request().cookies()).get(REVIEW_VOTER_COOKIE)?.value;
    const res = await gqlFetch(
      ReviewsPageDocument,
      { productId, sort },
      voterToken
        ? { extraHeaders: { "X-Review-Voter-Token": voterToken } }
        : undefined,
    );
    return res.data.product?.reviews ?? null;
  } catch {
    // Soft dependency - the caller keeps the list it already shows
    return null;
  }
}
