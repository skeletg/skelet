// Cookie-backed GDPR consent per category - nothing granted is the strict default. Server components read it here;
// server actions own writing it

import { cookies } from "next/headers";

export { CONSENT_COOKIE } from "@/app/lib/consent/cookie";
import { CONSENT_COOKIE } from "@/app/lib/consent/cookie";
import { NONE_GRANTED, parseConsent, type Consent } from "@/app/lib/consent/categories";

export type ResolvedConsent = Consent & { decided: boolean };

export async function getResolvedConsent(): Promise<ResolvedConsent> {
  const c = parseConsent((await cookies()).get(CONSENT_COOKIE)?.value);
  return { ...(c ?? NONE_GRANTED), decided: c !== null };
}
