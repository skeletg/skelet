"use server";

import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { revalidatePath } from "next/cache";

import {
  AddAddressDocument,
  FinalizeOrderPaymentDocument,
  PlaceOrderDocument,
  ValidateAddressDocument,
} from "@/app/lib/checkout/operations.gql";
import { ACCESS_TOKEN_COOKIE } from "@/app/lib/auth/session";
import { CART_COOKIE } from "@/app/lib/cart/read";
import { gqlFetch, GraphQLRequestError } from "@/app/lib/graphql";

// CheckoutResult mirrors the auth-actions / coupon-actions shape so the form-state hook on the checkout page can render
// an inline error without a route-segment error boundary
export type CheckoutResult =
  | { ok: true; addressId?: string }
  | {
      ok: true;
      requiresAction: { clientSecret: string; orderId: string; redirectTo: string };
    }
  | { ok: false; error: string };

function errorMessage(err: unknown, fallback: string): string {
  if (err instanceof GraphQLRequestError && err.errors[0]?.message) {
    return err.errors[0].message;
  }
  if (err instanceof Error && err.message) return err.message;
  return fallback;
}

export async function placeOrderAction(
  _prev: CheckoutResult | null,
  formData: FormData,
): Promise<CheckoutResult> {
  const email = String(formData.get("email") ?? "").trim().toLowerCase();
  if (!email) {
    return { ok: false, error: "Email is required." };
  }

  const billingAddressId = String(formData.get("billingAddressId") ?? "").trim();
  const shippingAddressId = String(
    formData.get("shippingAddressId") ?? "",
  ).trim();
  const customerNote = String(formData.get("customerNote") ?? "").trim();
  const shippingCarrierSlug = String(
    formData.get("shippingCarrierSlug") ?? "",
  ).trim();
  const shippingMethodCode = String(
    formData.get("shippingMethodCode") ?? "",
  ).trim();
  const paymentProviderSlug = String(
    formData.get("paymentProviderSlug") ?? "",
  ).trim();
  const paymentToken = String(formData.get("paymentToken") ?? "").trim();
  const giftCardCodes = String(formData.get("giftCardCodes") ?? "")
    .split(/[\n,]+/)
    .map((c) => c.trim())
    .filter((c) => c !== "");
  const useStoreCredit = String(formData.get("useStoreCredit") ?? "") === "1";
  // Saved payment method id (when the checkout UI offers stored cards); the backend
  // resolves it to the vaulted provider token
  const savedPaymentMethodId =
    String(formData.get("savedPaymentMethodId") ?? "").trim() || null;

// Inline addresses, namespaced billing.*/shipping.* to stay distinct from address-book ids. "Billing same as shipping"
// omits the billing fields, so mirror them here or the backend's per-slot validation fails
  const shippingInline = readInlineAddress(formData, "shipping");
  const billingSameAsShipping =
    String(formData.get("billingSameAsShipping") ?? "") === "1";
  const billingInline =
    billingSameAsShipping && shippingInline
      ? shippingInline
      : readInlineAddress(formData, "billing");
  // For logged-in customers with "same as shipping", mirror the ID too
  const effectiveBillingId =
    billingSameAsShipping && !billingInline
      ? shippingAddressId
      : billingAddressId;

  // At least one address source must be set per slot
  const hasBilling = !!effectiveBillingId || !!billingInline;
  const hasShipping = !!shippingAddressId || !!shippingInline;
  if (!hasBilling || !hasShipping) {
    return {
      ok: false,
      error: "Provide a billing and shipping address.",
    };
  }

  if (shippingInline) {
    try {
      const check = await gqlFetch(ValidateAddressDocument, {
        street1: shippingInline.street1,
        street2: shippingInline.street2 ?? null,
        city: shippingInline.city,
        state: shippingInline.state ?? null,
        postalCode: shippingInline.postalCode,
        countryCode: shippingInline.countryCode,
      });
      const v = check.data.validateAddress;
      if (!v.valid && v.strict) {
        return {
          ok: false,
          error: `Please check the shipping address: ${v.messages.join("; ")}.`,
        };
      }
      if (v.normalized) {
        shippingInline.street1 = v.normalized.street1;
        shippingInline.street2 = v.normalized.street2 || shippingInline.street2;
        shippingInline.city = v.normalized.city;
        shippingInline.state = v.normalized.state || shippingInline.state;
        shippingInline.postalCode = v.normalized.postalCode;
        shippingInline.countryCode = v.normalized.countryCode;
      }
    } catch {
      // Validator unavailable - never block checkout on it
    }
  }

  // Cart cookie: required for guests; logged-in customers can let the backend derive their active cart, but if a cookie
  // is set we still pass it (matches the cart the storefront has been showing)
  const cookieStore = await cookies();
  const cartId = cookieStore.get(CART_COOKIE)?.value ?? null;

  let orderID: string;
  let paymentClientSecret: string | null = null;
  try {
    const res = await gqlFetch(PlaceOrderDocument, {
      email,
      cartId: cartId || null,
      customerNote: customerNote || null,
      shippingCarrierSlug: shippingCarrierSlug || null,
      shippingMethodCode: shippingMethodCode || null,
      paymentProviderSlug: paymentProviderSlug || null,
      paymentToken: paymentToken || null,
      billingAddressId: effectiveBillingId || null,
      shippingAddressId: shippingAddressId || null,
      billingAddress: billingInline,
      shippingAddress: shippingInline,
      giftCardCodes: giftCardCodes.length > 0 ? giftCardCodes : null,
      useStoreCredit,
      savedPaymentMethodId,
    });
    orderID = res.data.placeOrder.id;
    paymentClientSecret = res.data.placeOrder.paymentClientSecret ?? null;
  } catch (err) {
    return { ok: false, error: errorMessage(err, "Could not place order.") };
  }

  cookieStore.delete(CART_COOKIE);
  revalidatePath("/", "layout");

  const isAuthed = cookieStore.get(ACCESS_TOKEN_COOKIE) !== undefined;
  const redirectTo = isAuthed
    ? `/account/orders/${orderID}?just_placed=1`
    : `/orders/${orderID}?email=${encodeURIComponent(email)}&just_placed=1`;

  if (paymentClientSecret) {
    return {
      ok: true,
      requiresAction: { clientSecret: paymentClientSecret, orderId: orderID, redirectTo },
    };
  }

  redirect(redirectTo);
}

export async function finalizeOrderPaymentAction(orderId: string): Promise<boolean> {
  try {
    const res = await gqlFetch(FinalizeOrderPaymentDocument, { orderId });
    return res.data.finalizeOrderPayment;
  } catch {
    return false;
  }
}
function readInlineAddress(
  formData: FormData,
  slot: "billing" | "shipping",
): {
  firstName: string;
  lastName: string;
  company: string | null;
  street1: string;
  street2: string | null;
  city: string;
  state: string | null;
  postalCode: string;
  countryCode: string;
  phone: string | null;
} | null {
  const street1 = String(formData.get(`${slot}.street1`) ?? "").trim();
  if (!street1) return null;
  return {
    firstName: String(formData.get(`${slot}.firstName`) ?? "").trim(),
    lastName: String(formData.get(`${slot}.lastName`) ?? "").trim(),
    company: optionalString(formData.get(`${slot}.company`)),
    street1,
    street2: optionalString(formData.get(`${slot}.street2`)),
    city: String(formData.get(`${slot}.city`) ?? "").trim(),
    state: optionalString(formData.get(`${slot}.state`)),
    postalCode: String(formData.get(`${slot}.postalCode`) ?? "").trim(),
    countryCode: String(formData.get(`${slot}.countryCode`) ?? "")
      .trim()
      .toUpperCase(),
    phone: optionalString(formData.get(`${slot}.phone`)),
  };
}
export async function addAddressAction(
  _prev: CheckoutResult | null,
  formData: FormData,
): Promise<CheckoutResult> {
  const input = {
    firstName: String(formData.get("firstName") ?? "").trim(),
    lastName: String(formData.get("lastName") ?? "").trim(),
    company: optionalString(formData.get("company")),
    street1: String(formData.get("street1") ?? "").trim(),
    street2: optionalString(formData.get("street2")),
    city: String(formData.get("city") ?? "").trim(),
    state: optionalString(formData.get("state")),
    postalCode: String(formData.get("postalCode") ?? "").trim(),
    countryCode: String(formData.get("countryCode") ?? "").trim().toUpperCase(),
    phone: optionalString(formData.get("phone")),
  };

  if (
    !input.firstName ||
    !input.lastName ||
    !input.street1 ||
    !input.city ||
    !input.postalCode ||
    !input.countryCode
  ) {
    return { ok: false, error: "Fill in name, street, city, postal, and country." };
  }
  if (input.countryCode.length !== 2) {
    return { ok: false, error: "Country code must be the 2-letter ISO form (US, DE, …)." };
  }

  let newID: string
  try {
    const res = await gqlFetch(AddAddressDocument, { input });
    newID = res.data.addAddress.id;
  } catch (err) {
    return { ok: false, error: errorMessage(err, "Could not save address.") };
  }
  // The checkout page reads MyProfile (server component); revalidate so the newly-added
  // address shows up in the picker
  revalidatePath("/checkout");
  return { ok: true, addressId: newID };
}

function optionalString(v: FormDataEntryValue | null): string | null {
  if (typeof v !== "string") return null;
  const trimmed = v.trim();
  return trimmed === "" ? null : trimmed;
}
