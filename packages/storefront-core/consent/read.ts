// Cookie-backed GDPR consent - "essential" (functional only) is the strict default, "all" additionally unlocks the
// dataLayer events in AnalyticsScripts Never read or written via document.cookie - server actions own it


export { CONSENT_COOKIE } from "./cookie";
import { request } from "../request";
import { CONSENT_COOKIE } from "./cookie";

export type ConsentChoice = "all" | "essential" | null;

export async function getConsentChoice(): Promise<ConsentChoice> {
  const store = await request().cookies();
  const raw = store.get(CONSENT_COOKIE)?.value;
  if (raw === "all" || raw === "essential") return raw;
  return null;
}

export type ResolvedConsent = {
  choice: ConsentChoice;
  decided: boolean;
  marketing: boolean;
};

export async function getResolvedConsent(): Promise<ResolvedConsent> {
  const choice = await getConsentChoice();
  return {
    choice,
    decided: choice !== null,
    marketing: choice === "all",
  };
}
