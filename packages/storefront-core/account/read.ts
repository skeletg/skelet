
import { perRequest } from "../request";
import { AccountCountsDocument } from "./counts.gql";
import { gqlFetch } from "../graphql";

export const getAccountCounts = perRequest(async () => {
  try {
    const { data } = await gqlFetch(AccountCountsDocument);
    return {
      wishlist: data.wishlistProductIds.length,
      offers: data.myCartOffers.length,
      newOffers: data.myCartOffers.filter((cart) => cart.offer?.state === "DRAFT" || cart.offer?.state === "SENT").length,
    };
  } catch (err) {
    console.error("account counts load failed", err);
    return { wishlist: 0, offers: 0, newOffers: 0 };
  }
});
