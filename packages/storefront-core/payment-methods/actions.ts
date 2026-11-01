import { redirect } from "../request";
import { revalidateAccount } from "../revalidate";
import { withLocalePrefix } from "../locale/path";
import { getCurrentLocale } from "../locale/read";

import { gqlFetch } from "../graphql";
import {
  DeleteMyPaymentMethodDocument,
  MyPaymentMethodsDocument,
} from "./operations.gql";

const PAYMENT_METHODS_PAGE = "/account/payment-methods";

export type SavedPaymentMethod = {
  id: string;
  providerSlug: string;
  brand: string;
  last4: string;
  expMonth: number;
  expYear: number;
};

export async function getMyPaymentMethods(): Promise<SavedPaymentMethod[]> {
  try {
    const res = await gqlFetch(MyPaymentMethodsDocument, {});
    return res.data.myPaymentMethods;
  } catch {
    return [];
  }
}

// Saving a card needs a gateway plugin to tokenize it in the browser first, so the
// vaulting action went with the plugins - the list and delete paths stay
export async function deletePaymentMethodAction(
  formData: FormData,
): Promise<void> {
  const id = String(formData.get("id") ?? "");
  if (!id) return;
  let ok = true;
  try {
    await gqlFetch(DeleteMyPaymentMethodDocument, { id });
  } catch (err) {
    // A card that silently refuses to go is the sort of thing people re-click forever
     
    console.error("payment method delete failed:", err);
    ok = false;
  }
  revalidateAccount();
  redirect(await withLocalePrefix(ok ? PAYMENT_METHODS_PAGE : `${PAYMENT_METHODS_PAGE}?error=delete`, await getCurrentLocale()));
}
