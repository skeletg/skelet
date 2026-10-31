"use server";

import { cookies } from "next/headers";
import { revalidatePath } from "next/cache";

import { CONSENT_COOKIE, CONSENT_COOKIE_OPTS } from "@/app/lib/consent/cookie";
import { ALL_GRANTED, NONE_GRANTED, serializeConsent, type Consent } from "@/app/lib/consent/categories";

// Cookie consent only - email and SMS marketing is its own explicit opt-in (sign-up, checkout, account), so accepting
// cookies never subscribes anyone to mail
async function setConsentAction(choice: Consent) {
  const store = await cookies();
  store.set(CONSENT_COOKIE, serializeConsent(choice), CONSENT_COOKIE_OPTS);
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
