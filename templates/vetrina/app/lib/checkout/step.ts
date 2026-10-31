// Which checkout step is open, worked out from what the cart still owes - so a reload lands on the first step the
// shopper hasn't finished, and an earlier step can be reopened with ?step= to change it. What each step owes is the
// backend's requirements list, read through asks.ts - the storefront never re-derives a rule placement enforces

import type { BusinessIdField } from "@/app/lib/countries/read";

import { blocking, isDeliveryRequirement, isDetailsRequirement, type Requirement } from "./asks";
import { choiceKey } from "./choice";
import type { CheckoutSettings } from "./settings";

// How the order arrives comes first, from a postcode alone - then who it's for, then payment. A cart of downloads has
// nothing to deliver and starts at details
export const CHECKOUT_STEPS = ["delivery", "details", "payment"] as const;

export type CheckoutStep = (typeof CHECKOUT_STEPS)[number];

export type StepStatus = "active" | "completed" | "pending";

export type CheckoutProgress = {
  // False for a digital-only cart - the delivery step isn't shown at all
  needsDelivery: boolean;
  deliveryDone: boolean;
  detailsDone: boolean;
};

// The business half of the details step - the backend checks it apart from the delivery requirements
export type BusinessState = {
  purchaseType: "PERSONAL" | "BUSINESS";
  company: string;
  businessIds: Record<string, string>;
  // The identifiers the company's country asks for
  businessIdFields: BusinessIdField[];
};

export function businessComplete(b: BusinessState, s: CheckoutSettings): boolean {
  if (!s.business || b.purchaseType !== "BUSINESS") return true;
  if (b.company === "") return false;
  return !b.businessIdFields.some((f) => f.required && !b.businessIds[f.key]);
}

export function progressOf(
  c: { fulfillment: string; destination: unknown; requirements: Requirement[] },
  businessOk: boolean,
): CheckoutProgress {
  const needsDelivery = c.fulfillment !== "NONE";
  return {
    needsDelivery,
    deliveryDone: !needsDelivery || (c.destination !== null && !c.requirements.some((r) => isDeliveryRequirement(r) && blocking(r))),
    detailsDone: businessOk && !c.requirements.some((r) => isDetailsRequirement(r) && blocking(r)),
  };
}

export function stepsFor(p: CheckoutProgress): readonly CheckoutStep[] {
  return p.needsDelivery ? CHECKOUT_STEPS : (["details", "payment"] as const);
}

function done(step: CheckoutStep, p: CheckoutProgress): boolean {
  switch (step) {
    case "delivery":
      return p.deliveryDone;
    case "details":
      return p.detailsDone;
    case "payment":
      return false;
  }
}

export function firstIncompleteStep(p: CheckoutProgress): CheckoutStep {
  return stepsFor(p).find((s) => !done(s, p)) ?? "payment";
}

// The requested step when it's one the shopper has already reached, else the first one still owed - a ?step= can
// reopen a finished step but never skip ahead
export function activeStep(p: CheckoutProgress, requested?: string | null): CheckoutStep {
  const steps = stepsFor(p);
  const first = firstIncompleteStep(p);
  const want = steps.find((s) => s === requested);
  if (want && steps.indexOf(want) <= steps.indexOf(first)) return want;
  return first;
}

export function stepStatus(step: CheckoutStep, active: CheckoutStep, p: CheckoutProgress): StepStatus {
  if (step === active) return "active";
  // Everything before the first owed step is done; past it nothing is, even if a later step happens to hold a value
  const steps = stepsFor(p);
  const first = steps.indexOf(firstIncompleteStep(p));
  return steps.indexOf(step) < first && done(step, p) ? "completed" : "pending";
}

export type ArrivalFormat = { weekday: "short"; day: "numeric"; month: "short"; timeZone: "UTC" };

// "Thu 3 Oct" for every choice the carriers give an arrival date, keyed by choiceKey. The backend counts the date on the
// store's calendar, weekends in or out by carrier, so it's read as the calendar day it names - midnight UTC, formatted in UTC
export function arrivalLabels(
  choices: readonly { optionId: string; carrierSlug: string; methodCode: string; estimatedArrival?: string | null }[],
  format: (date: Date, options: ArrivalFormat) => string,
): Record<string, string> {
  const out: Record<string, string> = {};
  for (const c of choices) {
    if (!c.estimatedArrival) continue;
    const date = new Date(`${c.estimatedArrival}T00:00:00Z`);
    if (Number.isNaN(date.getTime())) continue;
    out[choiceKey(c)] = format(date, { weekday: "short", day: "numeric", month: "short", timeZone: "UTC" });
  }
  return out;
}
