"use server";

import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { revalidatePath } from "next/cache";

import {
  AccountAddAddressDocument,
  AccountDeleteAddressDocument,
  AccountUpdateAddressDocument,
  ChangeCustomerPasswordDocument,
  CustomerLoginDocument,
  CustomerLogoutDocument,
  CustomerRegisterDocument,
  RequestPasswordResetDocument,
  ResendVerificationEmailDocument,
  ResetPasswordDocument,
  SetDefaultBillingAddressDocument,
  SetDefaultShippingAddressDocument,
  UpdateProfileDocument,
} from "@/app/lib/auth/operations.gql";
import { MergeGuestCartDocument } from "@/app/lib/cart/mutations.gql";
import {
  clearSessionCookies,
  getAccessToken,
  getRefreshToken,
  setSessionCookies,
} from "@/app/lib/auth/session";
import { CART_COOKIE } from "@/app/lib/cart/read";
import { CART_MERGE_FLAG, CART_MERGE_FAILED } from "@/app/lib/cart/merge-flag";
import { gqlFetch } from "@/app/lib/graphql";
import { safeReturnTo } from "@/app/lib/auth/return-to";
import {
  addressSchema,
  changePasswordSchema,
  forgotSchema,
  loginSchema,
  profileSchema,
  registerSchema,
  resetSchema,
} from "@/app/lib/auth/schemas";
import { parseForm, type FormResult } from "@/app/lib/forms/schema";
import { replyError } from "@/app/lib/forms/reply";

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
    // The store can require a verified address before issuing any session - the account exists but there are no tokens
    // to set, so route to "check your inbox" instead of pretending the sign-in failed
    verificationRequired =
      payload.verificationRequired || !payload.accessToken || !payload.refreshToken;
    if (!verificationRequired) {
      await setSessionCookies(payload.accessToken!, payload.refreshToken!);
    }
  } catch (err) {
    return replyError(submission, err, "registrationFailed", PASSWORD_FIELDS);
  }
  if (verificationRequired) {
    redirect("/account/login?verify=required");
  }
  const merged = await mergeGuestCartIfNeeded();
  revalidatePath("/", "layout");
  let dest = withFlag(returnTo ?? "/account", "welcome", "1");
  if (!merged) dest = withFlag(dest, CART_MERGE_FLAG, CART_MERGE_FAILED);
  redirect(dest);
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
    return replyError(submission, err, "signInFailed", PASSWORD_FIELDS);
  }
  const merged = await mergeGuestCartIfNeeded();
  revalidatePath("/", "layout");
  const dest = returnTo ?? "/account";
  redirect(merged ? dest : withFlag(dest, CART_MERGE_FLAG, CART_MERGE_FAILED));
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
  return submission.reply({ resetForm: true });
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
  redirect("/account/login?reset=ok");
}

export async function resendVerificationEmailAction(formData: FormData) {
  const returnTo = safeReturnTo(formData.get("returnTo")) ?? "/account";
  if (!(await getAccessToken())) {
    redirect(`/account/login?returnTo=${encodeURIComponent(returnTo)}`);
  }
  let sent = true;
  try {
    await gqlFetch(ResendVerificationEmailDocument);
  } catch {
    // Expired session / backend hiccup - surface a soft "try again" state
    sent = false;
  }
  const sep = returnTo.includes("?") ? "&" : "?";
  redirect(`${returnTo}${sep}resent=${sent ? "1" : "0"}`);
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
  revalidatePath("/account");
  return submission.reply({ resetForm: true });
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
  revalidatePath("/account");
  return submission.reply();
}

// Address actions live with the auth surface because every mutation here is gated on a logged-in customer; failures
// bubble back as the same form result the profile/password forms use

export async function addAddressAction(
  _prev: FormResult | undefined,
  formData: FormData,
): Promise<FormResult> {
  const submission = parseForm(formData, addressSchema);
  if (submission.status !== "success") return submission.reply();
  try {
    await gqlFetch(AccountAddAddressDocument, { input: submission.value });
  } catch (err) {
    return replyError(submission, err, "addressAddFailed");
  }
  revalidatePath("/account");
  revalidatePath("/account/addresses");
  redirect("/account/addresses");
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
      input: submission.value,
    });
  } catch (err) {
    return replyError(submission, err, "addressSaveFailed");
  }
  revalidatePath("/account");
  revalidatePath("/account/addresses");
  redirect("/account/addresses");
}

const ADDRESSES_PAGE = "/account/addresses";

// The address list re-renders from the server, so a failure that isn't announced looks
// exactly like a click that never registered
function finishAddressAction(ok: boolean, errorKey: string): never {
  revalidatePath("/account");
  revalidatePath(ADDRESSES_PAGE);
  redirect(ok ? ADDRESSES_PAGE : `${ADDRESSES_PAGE}?error=${errorKey}`);
}

export async function deleteAddressAction(formData: FormData) {
  const id = String(formData.get("id") ?? "");
  if (!id) redirect(ADDRESSES_PAGE);
  let ok = true;
  try {
    await gqlFetch(AccountDeleteAddressDocument, { id });
  } catch (err) {
     
    console.error("address delete failed:", err);
    ok = false;
  }
  finishAddressAction(ok, "delete");
}

export async function setDefaultBillingAction(formData: FormData) {
  const id = String(formData.get("id") ?? "");
  if (!id) redirect(ADDRESSES_PAGE);
  let ok = true;
  try {
    await gqlFetch(SetDefaultBillingAddressDocument, { id });
  } catch (err) {
     
    console.error("default billing address failed:", err);
    ok = false;
  }
  finishAddressAction(ok, "default");
}

export async function setDefaultShippingAction(formData: FormData) {
  const id = String(formData.get("id") ?? "");
  if (!id) redirect(ADDRESSES_PAGE);
  let ok = true;
  try {
    await gqlFetch(SetDefaultShippingAddressDocument, { id });
  } catch (err) {
     
    console.error("default shipping address failed:", err);
    ok = false;
  }
  finishAddressAction(ok, "default");
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
  redirect("/");
}
