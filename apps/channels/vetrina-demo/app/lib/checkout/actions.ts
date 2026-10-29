"use server";

import type { Submission } from "@conform-to/zod/v4";
import { cookies } from "next/headers";
import { redirect, RedirectType } from "next/navigation";
import { refresh, revalidatePath } from "next/cache";

import { AccountAddAddressDocument } from "@/app/lib/auth/operations.gql";
import { ACCESS_TOKEN_COOKIE } from "@/app/lib/auth/session";
import { ApplyCouponDocument, SetCartNoteDocument } from "@/app/lib/cart/mutations.gql";
import { CART_COOKIE } from "@/app/lib/cart/read";
import { getCheckoutCart } from "@/app/lib/cart/offer";
import { decodeOfferSession, OFFER_CHECKOUT_COOKIE, OFFER_COOKIE } from "@/app/lib/cart/offer-cookie";
import {
  ApplyCheckoutGiftCardDocument,
  PlaceOrderDocument,
  RemoveCheckoutGiftCardDocument,
  SetCheckoutDeliveryDocument,
  SetCheckoutDeliveryPointDocument,
  SetCheckoutDestinationDocument,
  SetCheckoutDetailsDocument,
  SetCheckoutPaymentMethodDocument,
  SetCheckoutSplitDeliveryDocument,
  SetCheckoutStoreCreditDocument,
  type AddressInput,
} from "@/app/lib/checkout/operations.gql";
import type { CheckoutDetailsInput } from "@/app/graphql/schema.types";
import { checkoutPaymentMethods, getCheckoutState } from "@/app/lib/checkout/read";
import { detailsAsks } from "@/app/lib/checkout/asks";
import {
  deliveryPointSchema,
  groupDeliverySchema,
  destinationSchema,
  splitDeliverySchema,
  detailsStepSchema,
  billingIsShown,
  type DetailsMode,
  giftCardSchema,
  paymentStepSchema,
  splitSelection,
  type DetailsValues,
} from "@/app/lib/checkout/schemas";
import { getCheckoutSettings } from "@/app/lib/config/read";
import { getCountries } from "@/app/lib/countries/read";
import { getCustomerOrNull } from "@/app/lib/customer/read";
import { errorMessage } from "@/app/lib/forms/reply";
import { parseForm, replyReset, type FormResult } from "@/app/lib/forms/schema";
import { gqlFetch, GraphQLRequestError } from "@/app/lib/graphql";
import { withLocalePrefix } from "@/app/lib/locale/path";
import { formatCouponError } from "@/app/lib/cart/coupon-errors";
import { couponSchema } from "@/app/lib/cart/schemas";
import { checkoutHref } from "./path";
import { getCurrentLocale } from "@/app/lib/locale/read";

// Delivery saves return to the delivery step for review; Continue opens details. Other step saves refresh
// from the cart, which remains the source of checkout values and requirements.

type AnySubmission = Submission<unknown, string[], unknown>;

// Backend codes whose message is better said by the storefront, in the shopper's language
const CODE_KEYS: Record<string, string> = {
  CHECKOUT_DESTINATION_REQUIRED: "checkoutDestinationRequired",
  CHECKOUT_POSTAL_CODE_REQUIRED: "postalCodeRequired",
  CHECKOUT_EMAIL_REQUIRED: "checkoutEmailRequired",
  CHECKOUT_PHONE_REQUIRED: "phoneRequired",
  CHECKOUT_COMPANY_REQUIRED: "companyRequired",
  CHECKOUT_BUSINESS_ID_REQUIRED: "checkoutBusinessIdRequired",
  CHECKOUT_BUSINESS_ID_INVALID: "businessIdInvalid",
  CHECKOUT_PICKUP_CONTACT_REQUIRED: "checkoutPickupContactRequired",
  CHECKOUT_SHIPPING_ADDRESS_REQUIRED: "checkoutAddressRequired",
  CHECKOUT_BILLING_ADDRESS_REQUIRED: "checkoutBillingRequired",
  CHECKOUT_SHIPPING_METHOD_REQUIRED: "shippingMethodRequired",
  CHECKOUT_DELIVERY_POINT_REQUIRED: "deliveryPointRequired",
  CHECKOUT_DELIVERY_GROUP_UNKNOWN: "deliveryGroupChanged",
  DELIVERY_POINT_NOT_FOUND: "deliveryPointUnavailable",
  DELIVERY_SOURCE_UNAVAILABLE: "deliverySourceUnavailable",
  CART_EMAIL_INVALID: "email",
  SHIPPING_METHOD_UNAVAILABLE: "shippingMethodUnavailable",
  // Unavailable-class like the address validator's, so the code is all that survives here too
  SHIPPING_QUOTE_FAILED: "shippingQuoteFailed",
  CHECKOUT_CONFLICT: "checkoutConflict",
  CHECKOUT_FIELD_TOO_LONG: "checkoutFieldTooLong",
  TERMS_NOT_ACCEPTED: "tosRequired",
  PAYMENT_METHOD_UNAVAILABLE: "paymentMethodUnavailable",
  PAYMENT_REQUIRED: "paymentMethodRequired",
  GIFT_CARD_INVALID: "giftCardInvalid",
  PHONE_INVALID: "phoneInvalid",
  PHONE_COUNTRY_NOT_ALLOWED: "phoneCountryNotAllowed",
  ADDRESS_INVALID: "addressInvalid",
  INSUFFICIENT_STOCK: "insufficientStock",
  BUNDLE_CHANGED: "bundleChanged",
  PRODUCT_NOT_PUBLISHED: "productUnavailable",
  PRODUCT_NOT_PURCHASABLE: "productUnavailable",
  ORDER_MINIMUM_NOT_MET: "orderMinimumNotMet",
  GUEST_CHECKOUT_DISABLED: "guestCheckoutDisabled",
  CART_OFFER_PAYMENT_NOT_ALLOWED: "offerPaymentNotAllowed",
  CART_OFFER_LOCKED: "offerLocked",
  CART_OFFER_EXPIRED: "offerExpired",
  CART_OFFER_REVOKED: "offerRevoked",
  // The front API scrubs an unavailable-class message, so the code is all that survives
  ADDRESS_VALIDATION_UNAVAILABLE: "addressValidationUnavailable",
};

function checkoutError(submission: AnySubmission, err: unknown, fallbackKey: string): FormResult {
  const code = err instanceof GraphQLRequestError ? err.errors[0]?.extensions?.code : undefined;
  const key = typeof code === "string" ? CODE_KEYS[code] : undefined;
  return submission.reply({ formErrors: [key ?? errorMessage(err, fallbackKey)] });
}

// The offer while the shopper is completing one, else their own cart
async function submittedCart(formData: FormData) {
  const id = String(formData.get("cartId") ?? "");
  return id ? getCheckoutCart(id) : null;
}

async function runStep(
  formData: FormData,
  submission: AnySubmission,
  fallbackKey: string,
  save: (cartId: string) => Promise<unknown>,
  stayOnDelivery = false,
): Promise<FormResult> {
  const cart = await submittedCart(formData);
  const cartId = cart?.id ?? null;
  if (!cartId) return submission.reply({ formErrors: ["checkoutCartMissing"] });
  try {
    await save(cartId);
  } catch (err) {
    return checkoutError(submission, err, fallbackKey);
  }
  if (stayOnDelivery) redirect(await withLocalePrefix(checkoutHref(cart?.offer ? cart.id : undefined, { step: "delivery" }), await getCurrentLocale()), RedirectType.replace);
  refresh();
  return submission.reply();
}

// Step 1a - the postcode the options are quoted for. Saving it re-renders the step with what that postcode is offered
export async function saveDestinationStep(
  _prev: FormResult | undefined,
  formData: FormData,
): Promise<FormResult> {
  const submission = parseForm(formData, destinationSchema);
  if (submission.status !== "success") return submission.reply();
  const { countryCode, postalCode } = submission.value;
  return runStep(formData, submission, "checkoutSaveFailed", (cartId) =>
    gqlFetch(SetCheckoutDestinationDocument, { cartId, countryCode, postalCode }),
    true,
  );
}

// Step 1b - deliver everything together, or choose for each group of lines that allow different deliveries
export async function saveSplitDeliveryAction(
  _prev: FormResult | undefined,
  formData: FormData,
): Promise<FormResult> {
  const submission = parseForm(formData, splitDeliverySchema);
  if (submission.status !== "success") return submission.reply();
  const split = submission.value.split === "split";
  return runStep(formData, submission, "checkoutSaveFailed", (cartId) => gqlFetch(SetCheckoutSplitDeliveryDocument, { cartId, split }), true);
}

// Step 1c - one group's delivery, one of the option and carrier-method choices quoted for it
export async function saveGroupDeliveryAction(
  _prev: FormResult | undefined,
  formData: FormData,
): Promise<FormResult> {
  const submission = parseForm(formData, groupDeliverySchema);
  if (submission.status !== "success") return submission.reply();
  const { optionId, carrierSlug, methodCode } = splitSelection(submission.value.selection);
  const groupKey = submission.value.groupKey;
  return runStep(formData, submission, "checkoutSaveFailed", (cartId) =>
    gqlFetch(SetCheckoutDeliveryDocument, { cartId, groupKey, optionId, carrierSlug, methodCode }),
    true,
  );
}

// The store or PO box a group's delivery goes to - re-read from its source by the backend
export async function saveDeliveryPointAction(
  _prev: FormResult | undefined,
  formData: FormData,
): Promise<FormResult> {
  const submission = parseForm(formData, deliveryPointSchema);
  if (submission.status !== "success") return submission.reply();
  const { groupKey, pointId } = submission.value;
  return runStep(formData, submission, "checkoutSaveFailed", (cartId) =>
    gqlFetch(SetCheckoutDeliveryPointDocument, { cartId, groupKey, pointId }),
    true,
  );
}

type LooseAddress = NonNullable<DetailsValues["shipping"]>;

// The refinement guarantees every required field when the choice is "new"
function addressInput(v: LooseAddress | undefined, phone: string | null): AddressInput {
  return {
    firstName: v?.firstName ?? "",
    lastName: v?.lastName ?? "",
    company: v?.company ?? null,
    street1: v?.street1 ?? "",
    street2: v?.street2 ?? null,
    city: v?.city ?? "",
    state: v?.state ?? null,
    postalCode: v?.postalCode ?? "",
    countryCode: v?.countryCode ?? "",
    phone,
  };
}

// Step 2 - one write for contact, business details and the addresses or pickup contact. The schema is rebuilt from the
// same settings and cart the form was, so the action refuses exactly what the form did
export async function saveDetailsStep(
  _prev: FormResult | undefined,
  formData: FormData,
): Promise<FormResult> {
  const cart = await submittedCart(formData);
  const cartId = cart?.id ?? null;
  const [settings, checkout, countries, customer] = await Promise.all([
    getCheckoutSettings(),
    cartId ? getCheckoutState(cartId) : null,
    getCountries(),
    getCustomerOrNull(),
  ]);
  const asks = detailsAsks(checkout?.requirements ?? []);
  const mode: DetailsMode = {
    asks,
    signedIn: customer !== null,
    countries,
    savedCountries: Object.fromEntries((customer?.addresses ?? []).map((a) => [a.id, a.countryCode])),
    destinationCountry: checkout?.destination?.countryCode ?? "",
  };
  const { signedIn } = mode;
  const submission = parseForm(formData, detailsStepSchema(settings, mode));
  if (submission.status !== "success") return submission.reply();
  if (!cartId || !checkout) return submission.reply({ formErrors: ["checkoutCartMissing"] });
  const v = submission.value;
  const business = settings.business && v.purchaseType === "business";

  const input: CheckoutDetailsInput = {
    email: v.email,
    phone: v.phone,
    purchaseType: business ? "BUSINESS" : "PERSONAL",
    company: business ? v.company : null,
    businessIds: business
      ? Object.entries(v.businessIds ?? {}).flatMap(([key, value]) => (value ? [{ key, value }] : []))
      : null,
    poNumber: business ? v.poNumber : null,
    billingSameAsShipping: false,
  };
  const someoneElse = asks.collector && settings.pickupSomeoneElse && v.pickupWho === "someoneElse";
  if (billingIsShown(asks, someoneElse, v)) {
    if (v.billingChoice === "new") input.billingAddress = addressInput(v.billing, null);
    else if (v.billingChoice) input.billingAddressId = v.billingChoice;
  }

  const chosenBefore = new Set(checkout.deliveryGroups.filter((g) => g.selection).map((g) => g.key));
  let deliveryDropped = false;
  try {
    if (asks.collector) {
      input.pickupContact = { firstName: v.pickupFirstName ?? "", lastName: v.pickupLastName ?? "", someoneElse };
    }
    if (asks.shipping) {
      input.billingSameAsShipping = !!v.billingSame;
      if (v.shippingChoice !== "new") {
        input.shippingAddressId = v.shippingChoice;
      } else if (signedIn && v.saveToBook) {
        // Saved first so the cart records where the snapshot came from
        const saved = await gqlFetch(AccountAddAddressDocument, { input: addressInput(v.shipping, v.phone) });
        input.shippingAddressId = saved.data.addAddress.id;
      } else {
        input.shippingAddress = addressInput(v.shipping, null);
      }
    }
    const res = await gqlFetch(SetCheckoutDetailsDocument, { cartId, input });
    deliveryDropped = res.data.setCartDetails.checkout.deliveryGroups.some((g) => chosenBefore.has(g.key) && !g.selection);
  } catch (err) {
    return checkoutError(submission, err, "checkoutSaveFailed");
  }
  if (deliveryDropped) {
    // The address moved the order somewhere a chosen delivery doesn't reach - back to step 1, saying why
    redirect(await withLocalePrefix(checkoutHref(cart?.offer ? cart.id : undefined, { notice: "delivery-changed" }), String(formData.get("locale") ?? "")));
  }
  refresh();
  return submission.reply();
}

export async function applyGiftCardAction(
  _prev: FormResult | undefined,
  formData: FormData,
): Promise<FormResult> {
  const submission = parseForm(formData, giftCardSchema);
  if (submission.status !== "success") return submission.reply();
  const cart = await submittedCart(formData);
  const cartId = cart?.id ?? null;
  if (!cartId) return submission.reply({ formErrors: ["checkoutCartMissing"] });
  try {
    await gqlFetch(ApplyCheckoutGiftCardDocument, { cartId, code: submission.value.code });
  } catch (err) {
    return checkoutError(submission, err, "giftCardInvalid");
  }
  refresh();
  return replyReset(submission);
}

// Removing a card and toggling store credit have no input to get wrong - plain form actions, and a failure just
// leaves the page showing what the cart still holds
export async function removeGiftCardAction(formData: FormData): Promise<void> {
  const code = String(formData.get("code") ?? "").trim();
  const cart = await submittedCart(formData);
  const cartId = cart?.id ?? null;
  if (!code || !cartId) return;
  try {
    await gqlFetch(RemoveCheckoutGiftCardDocument, { cartId, code });
  } catch {
    return;
  }
  refresh();
}

export async function setStoreCreditAction(formData: FormData): Promise<void> {
  const cart = await submittedCart(formData);
  const cartId = cart?.id ?? null;
  if (!cartId) return;
  try {
    await gqlFetch(SetCheckoutStoreCreditDocument, { cartId, use: formData.get("use") === "1" });
  } catch {
    return;
  }
  refresh();
}

export async function placeOrderAction(
  _prev: FormResult | undefined,
  formData: FormData,
): Promise<FormResult> {
  const cart = await submittedCart(formData);
  const cartId = cart?.id ?? null;
  const offer = cart?.offer ?? null;
  const [settings, checkout] = await Promise.all([
    getCheckoutSettings(),
    cartId ? getCheckoutState(cartId) : null,
  ]);
  // Nothing owed once gift cards and store credit cover the total, so no method is asked for
  const requireMethod = (checkout?.totals.amountDueCents ?? 1) > 0;
  const submission = parseForm(formData, paymentStepSchema({ requireMethod, consent: settings.termsConsent }));
  if (submission.status !== "success") return submission.reply();
  const v = submission.value;
  const locale = String(formData.get("locale") ?? "");
  if (!cartId || !checkout) return submission.reply({ formErrors: ["checkoutCartMissing"] });
  // The list the shopper picked from may be stale (country, offer or store settings changed) or the post forged - only a
  // method this checkout can still pay with may place an order
  if (requireMethod) {
    const methods = await checkoutPaymentMethods(checkout, cartId, offer?.paymentMethods);
    if (!methods.some((m) => m.providerSlug === v.providerSlug)) {
      return submission.reply({ formErrors: ["paymentMethodUnavailable"] });
    }
  }

  let orderId: string;
  let needsAction: boolean;
  try {
    // The payment step's own choices land on the cart first, so a refused placement keeps them for the retry
    if (v.providerSlug) {
      await gqlFetch(SetCheckoutPaymentMethodDocument, { cartId, providerSlug: v.providerSlug });
    }
    await gqlFetch(SetCartNoteDocument, { cartId, note: v.note ?? "" });
    const res = await gqlFetch(PlaceOrderDocument, {
      cartId,
      savedPaymentMethodId: v.savedPaymentMethodId,
      // The backend holds a checkbox store to the same tick the form asked for
      acceptTerms: v.tos === true,
    });
    orderId = res.data.placeOrder.id;
    needsAction = !!res.data.placeOrder.paymentClientSecret;
  } catch (err) {
    refresh(); // a step the backend says is missing reopens once the page re-reads the cart
    return checkoutError(submission, err, "placeOrderFailed");
  }

  const cookieStore = await cookies();
  if (offer) {
    // The offer is history - the shopper's own cart is untouched and takes over again
    if (decodeOfferSession(cookieStore.get(OFFER_COOKIE)?.value)?.cartId === cartId) {
      cookieStore.delete(OFFER_COOKIE);
      cookieStore.delete(OFFER_CHECKOUT_COOKIE);
    }
  } else {
    if (cookieStore.get(CART_COOKIE)?.value === cartId) cookieStore.delete(CART_COOKIE);
  }
  revalidatePath("/", "layout");

  // A guest offer places a guest order for its recipient, whoever is signed in on this browser
  const signedIn = cookieStore.get(ACCESS_TOKEN_COOKIE) !== undefined && !offer?.isGuest;
  const orderPath = signedIn
    ? `/account/orders/${orderId}?`
    : `/orders/${orderId}?email=${encodeURIComponent(checkout.email)}&`;
  // A payment action needs a gateway SDK this demo doesn't ship, so nothing finalizes it here: the order is shown as
  // created but unpaid, without the purchase event or thank-you a just-placed order gets
  redirect(await withLocalePrefix(`${orderPath}${needsAction ? "payment=incomplete" : "just_placed=1"}`, locale));
}


export async function applyCheckoutCouponAction(_prev: FormResult | undefined, formData: FormData): Promise<FormResult> {
  const submission = parseForm(formData, couponSchema);
  if (submission.status !== "success") return submission.reply();
  const cart = await submittedCart(formData);
  if (!cart) return submission.reply({ formErrors: ["checkoutCartMissing"] });
  try {
    await gqlFetch(ApplyCouponDocument, { cartId: cart.id, code: submission.value.code });
  } catch (err) {
    return submission.reply({ fieldErrors: { code: [formatCouponError(err)] } });
  }
  refresh();
  return replyReset(submission);
}
