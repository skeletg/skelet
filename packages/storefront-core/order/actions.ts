
import { redirect } from "../request";
import { getCustomerOrNull } from "../customer/read";
import { TrackOrderDocument } from "../checkout/operations.gql";
import { parseForm, type FormResult } from "../forms/schema";
import { replyError } from "../forms/reply";
import { gqlFetch } from "../graphql";
import { getCurrentLocale } from "../locale/read";
import { withLocalePrefix } from "../locale/path";
import { trackOrderSchema } from "./schemas";

// A guest order opens on the guest receipt page; an account's order only behind sign-in, where myOrder checks the owner
export async function trackOrderAction(
  _prev: FormResult | undefined,
  formData: FormData,
): Promise<FormResult> {
  const submission = parseForm(formData, trackOrderSchema);
  if (submission.status !== "success") return submission.reply();
  let found;
  try {
    found = (await gqlFetch(TrackOrderDocument, submission.value)).data.trackOrder;
  } catch (err) {
    return replyError(submission, err, "trackOrderFailed");
  }
  if (!found) return submission.reply({ formErrors: ["orderNotFound"] });

  const locale = await getCurrentLocale();
  if (!found.requiresLogin) {
    redirect(await withLocalePrefix(`/orders/${found.orderId}?email=${encodeURIComponent(found.email)}`, locale));
  }
  const accountOrder = `/account/orders/${found.orderId}`;
  if (await getCustomerOrNull()) redirect(await withLocalePrefix(accountOrder, locale));
  redirect(await withLocalePrefix(`/account/login?returnTo=${encodeURIComponent(accountOrder)}`, locale));
}
