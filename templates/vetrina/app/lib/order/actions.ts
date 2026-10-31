"use server";

import { redirect } from "next/navigation";

import { getCustomerOrNull } from "@/app/lib/customer/read";
import { TrackOrderDocument } from "@/app/lib/checkout/operations.gql";
import { parseForm, type FormResult } from "@/app/lib/forms/schema";
import { replyError } from "@/app/lib/forms/reply";
import { gqlFetch } from "@/app/lib/graphql";
import { getCurrentLocale } from "@/app/lib/locale/read";
import { withLocalePrefix } from "@/app/lib/locale/path";
import { trackOrderSchema } from "@/app/lib/order/schemas";

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
