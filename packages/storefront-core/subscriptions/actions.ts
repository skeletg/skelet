import { redirect } from "../request";
import { revalidateAccount } from "../revalidate";
import { withLocalePrefix } from "../locale/path";
import { getCurrentLocale } from "../locale/read";

import { gqlFetch } from "../graphql";
import {
  CancelSubscriptionDocument,
  SetSubscriptionPaymentMethodDocument,
} from "../customer/operations.gql";

const PAGE = "/account/subscriptions";

export async function cancelSubscriptionAction(formData: FormData): Promise<void> {
  const id = String(formData.get("id") ?? "");
  if (!id) return;
  let ok = true;
  try {
    await gqlFetch(CancelSubscriptionDocument, { id });
  } catch (err) {
    // Silence here left the customer looking at a still-active subscription with no clue
     
    console.error("subscription cancel failed:", err);
    ok = false;
  }
  revalidateAccount();
  redirect(await withLocalePrefix(ok ? `${PAGE}?done=canceled` : `${PAGE}?error=cancel`, await getCurrentLocale()));
}

export async function setSubscriptionPaymentMethodAction(
  formData: FormData,
): Promise<void> {
  const subscriptionId = String(formData.get("subscriptionId") ?? "");
  const paymentMethodId = String(formData.get("paymentMethodId") ?? "");
  if (!subscriptionId) return;
  let ok = true;
  try {
    await gqlFetch(SetSubscriptionPaymentMethodDocument, {
      subscriptionId,
      paymentMethodId: paymentMethodId || null,
    });
  } catch (err) {
     
    console.error("subscription card change failed:", err);
    ok = false;
  }
  revalidateAccount();
  redirect(await withLocalePrefix(ok ? `${PAGE}?done=card` : `${PAGE}?error=card`, await getCurrentLocale()));
}
