// Cookie-backed GDPR consent per category - nothing granted is the strict default. Server components read it here;
// server actions own writing it

export { CONSENT_COOKIE } from "./cookie";
import { request } from "../request";
import { CONSENT_COOKIE } from "./cookie";
import { NONE_GRANTED, parseConsent, type Consent } from "./categories";

export type ResolvedConsent = Consent & { decided: boolean };

export async function getResolvedConsent(): Promise<ResolvedConsent> {
  const c = parseConsent((await request().cookies()).get(CONSENT_COOKIE)?.value);
  return { ...(c ?? NONE_GRANTED), decided: c !== null };
}
