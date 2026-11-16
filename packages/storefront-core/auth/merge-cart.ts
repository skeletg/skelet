import { request } from "../request";
import { MergeGuestCartDocument } from "../cart/mutations.gql";
import { CART_COOKIE } from "../cart/read";
import { gqlFetch } from "../graphql";

// Returns false when the merge failed. The guest cart cookie is the customer's only handle on those lines, so it stays
// put - dropping it on a backend hiccup throws the basket away for good
export async function mergeGuestCartIfNeeded(): Promise<boolean> {
  const store = await request().cookies();
  const guestCartId = store.get(CART_COOKIE)?.value;
  if (!guestCartId) return true;
  try {
    await gqlFetch(MergeGuestCartDocument, { guestCartId });
  } catch (err) {
     
    console.error("guest cart merge failed:", err);
    return false;
  }
  store.delete(CART_COOKIE);
  return true;
}
