"use server";

import { cookies } from "next/headers";
import { revalidatePath } from "next/cache";

import { CONSENT_COOKIE } from "@/app/lib/consent/read";
import { ALL_GRANTED, NONE_GRANTED, serializeConsent, type Consent } from "@/app/lib/consent/categories";

// Cookie consent only - email and SMS marketing is its own explicit opt-in (sign-up, checkout, account), so accepting
// cookies never subscribes anyone to mail
async function setConsentAction(choice: Consent) {
  const store = await cookies();
  store.set(CONSENT_COOKIE, serializeConsent(choice), {
    path: "/",
    httpOnly: false, // the banner and the trackers resolve it in the browser, so cached pages need no server render
    sameSite: "lax",
    secure: process.env.NODE_ENV === "production",
    maxAge: 60 * 60 * 24 * 180, // 180 days; standard GDPR re-prompt window
  });
  // Refresh the layout so server-rendered consent reads see the new choice
  revalidatePath("/", "layout");
}

export async function acceptAllConsentAction() {
  await setConsentAction(ALL_GRANTED);
}

export async function essentialOnlyConsentAction() {
  await setConsentAction(NONE_GRANTED);
}

// The banner's per-category form - an unticked box is absent from the form data, which reads as denied
export async function saveConsentAction(form: FormData) {
  await setConsentAction({
    analytics: form.get("analytics") === "on",
    ads: form.get("ads") === "on",
    personalization: form.get("personalization") === "on",
  });
}
