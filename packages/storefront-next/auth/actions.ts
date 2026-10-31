"use server";

import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { withLocalePrefix } from "../locale/path";
import { getCurrentLocale } from "../locale/read";
import { revalidatePath } from "next/cache";

import { revalidateAccount } from "../revalidate";
import {
  AccountAddAddressDocument,
  AccountDeleteAddressDocument,
  AccountUpdateAddressDocument,
  ChangeCustomerPasswordDocument,
  CustomerLoginDocument,
  CustomerLogoutDocument,
  CustomerRegisterDocument,
  RequestPasswordResetDocument,
  ResendVerificationDocument,
  ResendVerificationEmailDocument,
  ResetPasswordDocument,
  SetDefaultBillingAddressDocument,
  SetDefaultShippingAddressDocument,
  UpdateProfileDocument,
} from "./operations.gql";
import { MergeGuestCartDocument } from "../cart/mutations.gql";
import {
  clearSessionCookies,
  getAccessToken,
  getRefreshToken,
  setSessionCookies,
} from "./session";
import { CART_COOKIE } from "../cart/read";
import { CART_MERGE_FLAG, CART_MERGE_FAILED } from "../cart/merge-flag";
import { GraphQLRequestError, gqlFetch } from "../graphql";
import { safeReturnTo } from "./return-to";
import {
  addressSchema,
  changePasswordSchema,
  forgotSchema,
  loginSchema,
  profileSchema,
  registerSchema,
  resetSchema,
} from "./schemas";
import { parseForm, replyReset, type FormResult } from "../forms/schema";
import { replyError } from "../forms/reply";

// Returns false when the merge failed. The guest cart cookie is the customer's only handle on those lines, so it stays
// put - dropping it on a backend hiccup throws the basket away for good
async function mergeGuestCartIfNeeded(): Promise<boolean> {
  const store = await cookies();
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

// withFlag appends a one-shot status param the destination page (or the global notice)
// can react to - the app's existing way of talking back across a redirect
function withFlag(path: string, key: string, value: string): string {
  const sep = path.includes("?") ? "&" : "?";
  return `${path}${sep}${key}=${value}`;
}

// Never echoed back into a re-rendered form - a failed submit clears them
const PASSWORD_FIELDS = [
  "password",
  "confirmPassword",
  "currentPassword",
  "newPassword",
];

export async function registerAction(
  _prev: FormResult | undefined,
  formData: FormData,
): Promise<FormResult> {
  const submission = parseForm(formData, registerSchema);
  if (submission.status !== "success") {
    return submission.reply({ hideFields: PASSWORD_FIELDS });
  }
  const { email, password, firstName, lastName, referralCode } = submission.value;
  const marketingOptIn = submission.value.marketingOptIn ?? false;

  const returnTo = safeReturnTo(formData.get("returnTo"));
  let verificationRequired = false;
  try {
    const res = await gqlFetch(CustomerRegisterDocument, {
      input: { email, password, firstName, lastName, marketingOptIn, referralCode },
    });
    const payload = res.data.customerRegister;
    // No tokens means the inbox holds the next step - a verification link, or a note to the owner when the address was
    // already registered. The two look the same here on purpose, so route both to "check your inbox"
    verificationRequired =
      payload.verificationRequired || !payload.accessToken || !payload.refreshToken;
    if (!verificationRequired) {
      await setSessionCookies(payload.accessToken!, payload.refreshToken!);
    }
  } catch (err) {
    return replyError(submission, err, "registrationFailed", PASSWORD_FIELDS);
  }
  if (verificationRequired) {
    redirect(await withLocalePrefix("/account/login?verify=required", await getCurrentLocale()));
  }
  const merged = await mergeGuestCartIfNeeded();
  revalidatePath("/", "layout");
  let dest = withFlag(returnTo ?? "/account", "welcome", "1");
  if (!merged) dest = withFlag(dest, CART_MERGE_FLAG, CART_MERGE_FAILED);
  redirect(await withLocalePrefix(dest, await getCurrentLocale()));
}

export async function loginAction(
  _prev: FormResult | undefined,
  formData: FormData,
): Promise<FormResult> {
  const submission = parseForm(formData, loginSchema);
  if (submission.status !== "success") {
    return submission.reply({ hideFields: PASSWORD_FIELDS });
  }
  const { email, password } = submission.value;

  const returnTo = safeReturnTo(formData.get("returnTo"));
  try {
    const res = await gqlFetch(CustomerLoginDocument, {
      input: { email, password },
    });
    const payload = res.data.customerLogin;
    // Login always carries tokens - the nullable shape is shared with register, which withholds them pending verification
    if (!payload.accessToken || !payload.refreshToken) {
      return submission.reply({
        formErrors: ["signInFailed"],
        hideFields: PASSWORD_FIELDS,
      });
    }
    await setSessionCookies(payload.accessToken, payload.refreshToken);
  } catch (err) {
    // The form offers a fresh confirmation link off this key, since the gate keeps the shopper from the signed-in resend
    if (err instanceof GraphQLRequestError && err.errors[0]?.extensions?.code === "EMAIL_NOT_VERIFIED") {
      return submission.reply({ formErrors: ["emailNotVerified"], hideFields: PASSWORD_FIELDS });
    }
    return replyError(submission, err, "signInFailed", PASSWORD_FIELDS);
  }
  const merged = await mergeGuestCartIfNeeded();
  revalidatePath("/", "layout");
  const dest = returnTo ?? "/account";
  redirect(await withLocalePrefix(merged ? dest : withFlag(dest, CART_MERGE_FLAG, CART_MERGE_FAILED), await getCurrentLocale()));
}

export async function requestPasswordResetAction(
  _prev: FormResult | undefined,
  formData: FormData,
): Promise<FormResult> {
  const submission = parseForm(formData, forgotSchema);
  if (submission.status !== "success") return submission.reply();
  try {
    await gqlFetch(RequestPasswordResetDocument, {
      email: submission.value.email,
    });
  } catch {
    // Same anti-enumeration story - surface success even if the backend returned an
    // error so we don't leak account existence
  }
  return replyReset(submission);
}

// token arrives as a bound argument, never as a form field - Next encrypts bound args,
// so the live reset secret stays out of the DOM (and out of replay/snapshot capture)
export async function resetPasswordAction(
  boundToken: string,
  _prev: FormResult | undefined,
  formData: FormData,
): Promise<FormResult> {
  const submission = parseForm(formData, resetSchema);
  if (submission.status !== "success") {
    return submission.reply({ hideFields: PASSWORD_FIELDS });
  }
  const token = boundToken.trim();
  if (!token) {
    return submission.reply({
      formErrors: ["resetLinkMissing"],
      hideFields: PASSWORD_FIELDS,
    });
  }
  const { newPassword } = submission.value;
  try {
    await gqlFetch(ResetPasswordDocument, { token, newPassword });
  } catch (err) {
    return replyError(submission, err, "resetLinkInvalid", PASSWORD_FIELDS);
  }
  redirect(await withLocalePrefix("/account/login?reset=ok", await getCurrentLocale()));
}

export async function resendVerificationEmailAction(formData: FormData) {
  const returnTo = safeReturnTo(formData.get("returnTo")) ?? "/account";
  if (!(await getAccessToken())) {
    redirect(await withLocalePrefix(`/account/login?returnTo=${encodeURIComponent(returnTo)}`, await getCurrentLocale()));
  }
  let sent = true;
  try {
    await gqlFetch(ResendVerificationEmailDocument);
  } catch {
    // Expired session / backend hiccup - surface a soft "try again" state
    sent = false;
  }
  const sep = returnTo.includes("?") ? "&" : "?";
  redirect(await withLocalePrefix(`${returnTo}${sep}resent=${sent ? "1" : "0"}`, await getCurrentLocale()));
}

// Signed out, for an account the verification gate won't let sign in. The backend answers the same for any address, and
// so does this - a failure is not shown, or it would say the address exists
export async function resendVerificationAction(formData: FormData) {
  const email = String(formData.get("email") ?? "").trim();
  const returnTo = safeReturnTo(formData.get("returnTo"));
  if (email) {
    try {
      await gqlFetch(ResendVerificationDocument, { email });
    } catch (err) {
      console.error("resend verification failed:", err);
    }
  }
  const dest = "/account/login?verify=sent";
  redirect(await withLocalePrefix(returnTo ? `${dest}&returnTo=${encodeURIComponent(returnTo)}` : dest, await getCurrentLocale()));
}

export async function changePasswordAction(
  _prev: FormResult | undefined,
  formData: FormData,
): Promise<FormResult> {
  const submission = parseForm(formData, changePasswordSchema);
  if (submission.status !== "success") {
    return submission.reply({ hideFields: PASSWORD_FIELDS });
  }
  const { currentPassword, newPassword } = submission.value;
  try {
    await gqlFetch(ChangeCustomerPasswordDocument, {
      currentPassword,
      newPassword,
    });
  } catch (err) {
    return replyError(submission, err, "passwordChangeFailed", PASSWORD_FIELDS);
  }
  revalidateAccount();
  return replyReset(submission);
}

export async function updateProfileAction(
  _prev: FormResult | undefined,
  formData: FormData,
): Promise<FormResult> {
  const submission = parseForm(formData, profileSchema);
  if (submission.status !== "success") return submission.reply();
  // phone is null when cleared, so the backend wipes the column
  const { firstName, lastName, phone } = submission.value;
  try {
    await gqlFetch(UpdateProfileDocument, { firstName, lastName, phone });
  } catch (err) {
    return replyError(submission, err, "profileSaveFailed");
  }
  revalidateAccount();
  return submission.reply();
}

// Address actions live with the auth surface because every mutation here is gated on a logged-in customer; failures
// bubble back as the same form result the profile/password forms use

// The form posts PhoneInput's hidden companions beside the address - the API takes the address alone
function addressInput<T extends { phoneCountry?: unknown; phoneAllowed?: unknown }>(v: T) {
  const { phoneCountry: _country, phoneAllowed: _allowed, ...address } = v;
  return address;
}

export async function addAddressAction(
  _prev: FormResult | undefined,
  formData: FormData,
): Promise<FormResult> {
  const submission = parseForm(formData, addressSchema);
  if (submission.status !== "success") return submission.reply();
  try {
    await gqlFetch(AccountAddAddressDocument, { input: addressInput(submission.value) });
  } catch (err) {
    return replyError(submission, err, "addressAddFailed");
  }
  revalidateAccount();
  redirect(await withLocalePrefix("/account/addresses", await getCurrentLocale()));
}

export async function updateAddressAction(
  _prev: FormResult | undefined,
  formData: FormData,
): Promise<FormResult> {
  const submission = parseForm(formData, addressSchema);
  if (submission.status !== "success") return submission.reply();
  const id = String(formData.get("id") ?? "");
  if (!id) return submission.reply({ formErrors: ["addressIdMissing"] });
  try {
    await gqlFetch(AccountUpdateAddressDocument, {
      id,
      input: addressInput(submission.value),
    });
  } catch (err) {
    return replyError(submission, err, "addressSaveFailed");
  }
  revalidateAccount();
  redirect(await withLocalePrefix("/account/addresses", await getCurrentLocale()));
}

const ADDRESSES_PAGE = "/account/addresses";

// The address list re-renders from the server, so a failure that isn't announced looks
// exactly like a click that never registered
async function finishAddressAction(ok: boolean, errorKey: string): Promise<never> {
  revalidateAccount();
  redirect(await withLocalePrefix(ok ? ADDRESSES_PAGE : `${ADDRESSES_PAGE}?error=${errorKey}`, await getCurrentLocale()));
}

export async function deleteAddressAction(formData: FormData) {
  const id = String(formData.get("id") ?? "");
  if (!id) redirect(await withLocalePrefix(ADDRESSES_PAGE, await getCurrentLocale()));
  let ok = true;
  try {
    await gqlFetch(AccountDeleteAddressDocument, { id });
  } catch (err) {
     
    console.error("address delete failed:", err);
    ok = false;
  }
  await finishAddressAction(ok, "delete");
}

export async function setDefaultBillingAction(formData: FormData) {
  const id = String(formData.get("id") ?? "");
  if (!id) redirect(await withLocalePrefix(ADDRESSES_PAGE, await getCurrentLocale()));
  let ok = true;
  try {
    await gqlFetch(SetDefaultBillingAddressDocument, { id });
  } catch (err) {
     
    console.error("default billing address failed:", err);
    ok = false;
  }
  await finishAddressAction(ok, "default");
}

export async function setDefaultShippingAction(formData: FormData) {
  const id = String(formData.get("id") ?? "");
  if (!id) redirect(await withLocalePrefix(ADDRESSES_PAGE, await getCurrentLocale()));
  let ok = true;
  try {
    await gqlFetch(SetDefaultShippingAddressDocument, { id });
  } catch (err) {
     
    console.error("default shipping address failed:", err);
    ok = false;
  }
  await finishAddressAction(ok, "default");
}

export async function logoutAction() {
  const refresh = await getRefreshToken();
  if (refresh) {
    try {
      await gqlFetch(CustomerLogoutDocument, { token: refresh });
    } catch {
      // Even if the backend rejects (token already expired / never existed), we still
      // wipe the local cookies
    }
  }
  await clearSessionCookies();
  revalidatePath("/", "layout");
  redirect(await withLocalePrefix("/", await getCurrentLocale()));
}
