"use server";

import type { Submission } from "@conform-to/zod/v4";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { refresh, revalidatePath } from "next/cache";

import { AccountAddAddressDocument } from "@/app/lib/auth/operations.gql";
import { ACCESS_TOKEN_COOKIE } from "@/app/lib/auth/session";
import { SetCartNoteDocument } from "@/app/lib/cart/mutations.gql";
import { CART_COOKIE, getCart } from "@/app/lib/cart/read";
import {
  ApplyCheckoutGiftCardDocument,
  FinalizeOrderPaymentDocument,
  PlaceOrderDocument,
  RemoveCheckoutGiftCardDocument,
  SetCheckoutDestinationDocument,
  SetCheckoutDetailsDocument,
  SetCheckoutPaymentMethodDocument,
  SetCheckoutShippingMethodDocument,
  SetCheckoutStoreCreditDocument,
  type AddressInput,
} from "@/app/lib/checkout/operations.gql";
import type { CheckoutDetailsInput } from "@/app/graphql/schema.types";
import { getCheckoutState } from "@/app/lib/checkout/read";
import {
  deliveryStepSchema,
  destinationSchema,
  detailsStepSchema,
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

// Every step saves to the cart and refreshes the page, which re-reads the cart and opens the next step - the cart is
// the checkout's only state, so a reload lands in the same place

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
  ORDER_MINIMUM_NOT_MET: "orderMinimumNotMet",
  GUEST_CHECKOUT_DISABLED: "guestCheckoutDisabled",
  // The front API scrubs an unavailable-class message, so the code is all that survives
  ADDRESS_VALIDATION_UNAVAILABLE: "addressValidationUnavailable",
};

function checkoutError(submission: AnySubmission, err: unknown, fallbackKey: string): FormResult {
  const code = err instanceof GraphQLRequestError ? err.errors[0]?.extensions?.code : undefined;
  const key = typeof code === "string" ? CODE_KEYS[code] : undefined;
  return submission.reply({ formErrors: [key ?? errorMessage(err, fallbackKey)] });
}

async function currentCartId(): Promise<string | null> {
  return (await getCart())?.id ?? null;
}

async function runStep(
  submission: AnySubmission,
  fallbackKey: string,
  save: (cartId: string) => Promise<unknown>,
): Promise<FormResult> {
  const cartId = await currentCartId();
  if (!cartId) return submission.reply({ formErrors: ["checkoutCartMissing"] });
  try {
    await save(cartId);
  } catch (err) {
    return checkoutError(submission, err, fallbackKey);
  }
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
  return runStep(submission, "checkoutSaveFailed", (cartId) =>
    gqlFetch(SetCheckoutDestinationDocument, { cartId, countryCode, postalCode }),
  );
}

// Step 1b - one of the delivery or pickup options quoted for the destination
export async function saveDeliveryStep(
  _prev: FormResult | undefined,
  formData: FormData,
): Promise<FormResult> {
  const submission = parseForm(formData, deliveryStepSchema);
  if (submission.status !== "success") return submission.reply();
  const { carrierSlug, code } = splitSelection(submission.value.selection);
  return runStep(submission, "checkoutSaveFailed", (cartId) =>
    gqlFetch(SetCheckoutShippingMethodDocument, { cartId, carrierSlug, code }),
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
  const cartId = await currentCartId();
  const [settings, checkout, countries, customer] = await Promise.all([
    getCheckoutSettings(),
    cartId ? getCheckoutState(cartId) : null,
    getCountries(),
    getCustomerOrNull(),
  ]);
  const mode: DetailsMode = {
    pickup: checkout?.isPickup ?? false,
    signedIn: customer !== null,
    countries,
    savedCountries: Object.fromEntries((customer?.addresses ?? []).map((a) => [a.id, a.countryCode])),
    destinationCountry: checkout?.destination?.countryCode ?? "",
  };
  const { pickup, signedIn } = mode;
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
  const wantsBilling = pickup ? !!v.billingChoice : !v.billingSame;
  if (wantsBilling && v.billingChoice === "new") input.billingAddress = addressInput(v.billing, null);
  else if (wantsBilling && v.billingChoice) input.billingAddressId = v.billingChoice;

  let methodDropped = false;
  try {
    if (pickup) {
      input.pickupContact = {
        firstName: v.pickupFirstName ?? "",
        lastName: v.pickupLastName ?? "",
        someoneElse: settings.pickupSomeoneElse && v.pickupWho === "someoneElse",
      };
    } else {
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
    methodDropped = !!checkout.shippingMethod && !res.data.setCartDetails.checkout.shippingMethod;
  } catch (err) {
    return checkoutError(submission, err, "checkoutSaveFailed");
  }
  if (methodDropped) {
    // The address moved the order somewhere the chosen option doesn't reach - back to step 1, saying why
    redirect(await withLocalePrefix("/checkout?notice=delivery-changed", String(formData.get("locale") ?? "")));
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
  const cartId = await currentCartId();
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
  const cartId = await currentCartId();
  if (!code || !cartId) return;
  try {
    await gqlFetch(RemoveCheckoutGiftCardDocument, { cartId, code });
  } catch {
    return;
  }
  refresh();
}

export async function setStoreCreditAction(formData: FormData): Promise<void> {
  const cartId = await currentCartId();
  if (!cartId) return;
  try {
    await gqlFetch(SetCheckoutStoreCreditDocument, { cartId, use: formData.get("use") === "1" });
  } catch {
    return;
  }
  refresh();
}

export type PlaceOrderResult = FormResult & {
  // The order exists but the gateway wants a client-side challenge before it settles
  requiresAction?: { orderId: string; redirectTo: string };
};

export async function placeOrderAction(
  _prev: PlaceOrderResult | undefined,
  formData: FormData,
): Promise<PlaceOrderResult> {
  const cartId = await currentCartId();
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
  cookieStore.delete(CART_COOKIE);
  revalidatePath("/", "layout");

  const signedIn = cookieStore.get(ACCESS_TOKEN_COOKIE) !== undefined;
  const redirectTo = await withLocalePrefix(
    signedIn
      ? `/account/orders/${orderId}?just_placed=1`
      : `/orders/${orderId}?email=${encodeURIComponent(checkout.email)}&just_placed=1`,
    locale,
  );
  if (needsAction) {
    return { ...submission.reply(), requiresAction: { orderId, redirectTo } };
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
