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
import { gqlFetch, GraphQLRequestError } from "@/app/lib/graphql";
import { safeReturnTo } from "@/app/lib/auth/return-to";

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

// Server actions can't throw structured field errors to a form action, so return a
// serializable result instead - one error string is enough for login/register
export type AuthResult =
  | { ok: true }
  | { ok: false; error: string };

function errorMessage(err: unknown, fallback: string): string {
  if (err instanceof GraphQLRequestError && err.errors[0]?.message) {
    return err.errors[0].message;
  }
  if (err instanceof Error && err.message) return err.message;
  return fallback;
}

export async function registerAction(
  _prev: AuthResult | null,
  formData: FormData,
): Promise<AuthResult> {
  const email = String(formData.get("email") ?? "").trim();
  const password = String(formData.get("password") ?? "");
  const confirm = String(formData.get("confirmPassword") ?? "");
  const firstName = String(formData.get("firstName") ?? "").trim();
  const lastName = String(formData.get("lastName") ?? "").trim();
  const marketingOptIn = formData.get("marketingOptIn") === "on";
  const referralCode =
    String(formData.get("referralCode") ?? "").trim() || null;

  if (!email || !password || !firstName || !lastName) {
    return { ok: false, error: "All fields are required." };
  }
  if (password.length < 8) {
    return { ok: false, error: "Password must be at least 8 characters." };
  }
  if (password !== confirm) {
    return { ok: false, error: "Passwords do not match." };
  }

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
    return { ok: false, error: errorMessage(err, "Registration failed.") };
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
  _prev: AuthResult | null,
  formData: FormData,
): Promise<AuthResult> {
  const email = String(formData.get("email") ?? "").trim();
  const password = String(formData.get("password") ?? "");

  if (!email || !password) {
    return { ok: false, error: "Email and password are required." };
  }

  const returnTo = safeReturnTo(formData.get("returnTo"));
  try {
    const res = await gqlFetch(CustomerLoginDocument, {
      input: { email, password },
    });
    const payload = res.data.customerLogin;
    // Login always carries tokens - the nullable shape is shared with register, which withholds them pending verification
    if (!payload.accessToken || !payload.refreshToken) {
      return { ok: false, error: "Sign in failed." };
    }
    await setSessionCookies(payload.accessToken, payload.refreshToken);
  } catch (err) {
    return { ok: false, error: errorMessage(err, "Sign in failed.") };
  }
  const merged = await mergeGuestCartIfNeeded();
  revalidatePath("/", "layout");
  const dest = returnTo ?? "/account";
  redirect(merged ? dest : withFlag(dest, CART_MERGE_FLAG, CART_MERGE_FAILED));
}

export async function requestPasswordResetAction(
  _prev: AuthResult | null,
  formData: FormData,
): Promise<AuthResult> {
  const email = String(formData.get("email") ?? "").trim();
  if (!email) {
    return { ok: false, error: "Email is required." };
  }
  try {
    await gqlFetch(RequestPasswordResetDocument, { email });
  } catch {
    // Same anti-enumeration story - surface success even if the backend returned an
    // error so we don't leak account existence
  }
  return { ok: true };
}

// token arrives as a bound argument, never as a form field - Next encrypts bound args,
// so the live reset secret stays out of the DOM (and out of replay/snapshot capture)
export async function resetPasswordAction(
  boundToken: string,
  _prev: AuthResult | null,
  formData: FormData,
): Promise<AuthResult> {
  const token = boundToken.trim();
  const newPassword = String(formData.get("newPassword") ?? "");
  const confirm = String(formData.get("confirmPassword") ?? "");
  if (!token) return { ok: false, error: "Reset link is missing." };
  if (newPassword.length < 8) {
    return {
      ok: false,
      error: "Password must be at least 8 characters.",
    };
  }
  if (newPassword !== confirm) {
    return { ok: false, error: "Passwords do not match." };
  }
  try {
    await gqlFetch(ResetPasswordDocument, { token, newPassword });
  } catch (err) {
    return {
      ok: false,
      error: errorMessage(err, "Reset link is invalid or expired."),
    };
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
  _prev: AuthResult | null,
  formData: FormData,
): Promise<AuthResult> {
  const currentPassword = String(formData.get("currentPassword") ?? "");
  const newPassword = String(formData.get("newPassword") ?? "");
  const confirm = String(formData.get("confirmPassword") ?? "");
  if (!currentPassword || !newPassword) {
    return { ok: false, error: "All fields are required." };
  }
  if (newPassword.length < 8) {
    return {
      ok: false,
      error: "Password must be at least 8 characters.",
    };
  }
  if (newPassword !== confirm) {
    return { ok: false, error: "Passwords do not match." };
  }
  try {
    await gqlFetch(ChangeCustomerPasswordDocument, {
      currentPassword,
      newPassword,
    });
  } catch (err) {
    return {
      ok: false,
      error: errorMessage(err, "Could not change password."),
    };
  }
  revalidatePath("/account");
  return { ok: true };
}

export async function updateProfileAction(
  _prev: AuthResult | null,
  formData: FormData,
): Promise<AuthResult> {
  const firstName = String(formData.get("firstName") ?? "").trim();
  const lastName = String(formData.get("lastName") ?? "").trim();
  const phoneRaw = String(formData.get("phone") ?? "").trim();
  // Send null for an explicitly-cleared phone so the backend can wipe the column
  const phone = phoneRaw.length > 0 ? phoneRaw : null;
  try {
    await gqlFetch(UpdateProfileDocument, {
      firstName: firstName || null,
      lastName: lastName || null,
      phone,
    });
  } catch (err) {
    return { ok: false, error: errorMessage(err, "Could not save profile.") };
  }
  revalidatePath("/account");
  return { ok: true };
}

// Address actions live with the auth surface because every mutation here is gated on a logged-in customer; failures
// bubble back as the same AuthResult shape the profile/password forms use

type AddressFormShape = {
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
};

function readAddressInput(formData: FormData): AddressFormShape | string {
  const firstName = String(formData.get("firstName") ?? "").trim();
  const lastName = String(formData.get("lastName") ?? "").trim();
  const street1 = String(formData.get("street1") ?? "").trim();
  const city = String(formData.get("city") ?? "").trim();
  const postalCode = String(formData.get("postalCode") ?? "").trim();
  const countryCode = String(formData.get("countryCode") ?? "")
    .trim()
    .toUpperCase();
  if (
    !firstName ||
    !lastName ||
    !street1 ||
    !city ||
    !postalCode ||
    !countryCode
  ) {
    return "Please fill in every required field.";
  }
  // Country code must be a 2-letter ISO 3166 - backend validates the same shape;
  // catching it early gives a clearer error than the backend's generic "invalid input"
  if (!/^[A-Z]{2}$/.test(countryCode)) {
    return "Country must be a 2-letter code (e.g. US, DE, RS).";
  }
  const company = String(formData.get("company") ?? "").trim() || null;
  const street2 = String(formData.get("street2") ?? "").trim() || null;
  const state = String(formData.get("state") ?? "").trim() || null;
  const phone = String(formData.get("phone") ?? "").trim() || null;
  return {
    firstName,
    lastName,
    company,
    street1,
    street2,
    city,
    state,
    postalCode,
    countryCode,
    phone,
  };
}

export async function addAddressAction(
  _prev: AuthResult | null,
  formData: FormData,
): Promise<AuthResult> {
  const parsed = readAddressInput(formData);
  if (typeof parsed === "string") return { ok: false, error: parsed };
  try {
    await gqlFetch(AccountAddAddressDocument, { input: parsed });
  } catch (err) {
    return {
      ok: false,
      error: errorMessage(err, "Could not add address."),
    };
  }
  revalidatePath("/account");
  revalidatePath("/account/addresses");
  redirect("/account/addresses");
}

export async function updateAddressAction(
  _prev: AuthResult | null,
  formData: FormData,
): Promise<AuthResult> {
  const id = String(formData.get("id") ?? "");
  if (!id) return { ok: false, error: "Missing address id." };
  const parsed = readAddressInput(formData);
  if (typeof parsed === "string") return { ok: false, error: parsed };
  try {
    await gqlFetch(AccountUpdateAddressDocument, { id, input: parsed });
  } catch (err) {
    return {
      ok: false,
      error: errorMessage(err, "Could not save address."),
    };
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
