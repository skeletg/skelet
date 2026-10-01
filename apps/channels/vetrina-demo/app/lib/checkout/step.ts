// Which checkout step is open, worked out from what the cart already holds - so a reload lands on the first step the
// shopper still owes, and an earlier step can be reopened with ?step= to change it

import type { BusinessIdField } from "@/app/lib/countries/read";

import { pickupNeedsBilling, type CheckoutSettings } from "./settings";

// How the order arrives comes first, from a postcode alone - then who it's for, then payment
export const CHECKOUT_STEPS = ["delivery", "details", "payment"] as const;

export type CheckoutStep = (typeof CHECKOUT_STEPS)[number];

export type StepStatus = "active" | "completed" | "pending";

export type CheckoutProgress = {
  hasDestination: boolean;
  // Null on the cart when none was chosen, or when the chosen one stopped being offered (destination or lines changed)
  hasShippingMethod: boolean;
  detailsComplete: boolean;
};

// The slice of the cart's checkout the details step is judged on
export type DetailsState = {
  email: string;
  phone: string;
  purchaseType: "PERSONAL" | "BUSINESS";
  company: string;
  businessIds: Record<string, string>;
  // The identifiers the company's country asks for
  businessIdFields: BusinessIdField[];
  isPickup: boolean;
  hasShippingAddress: boolean;
  billingSameAsShipping: boolean;
  hasBillingAddress: boolean;
  pickupContact: { someoneElse: boolean } | null;
};

// The same rules the backend's detailsComplete applies at placement, so the page never shows a step done that
// placement would send the shopper back to
export function detailsComplete(d: DetailsState, s: CheckoutSettings): boolean {
  if (d.email === "") return false;
  if (s.phone === "required" && d.phone === "") return false;
  if (s.business && d.purchaseType === "BUSINESS") {
    if (d.company === "") return false;
    if (d.businessIdFields.some((f) => f.required && !d.businessIds[f.key])) return false;
  }
  if (d.isPickup) {
    if (!d.pickupContact) return false;
    return !pickupNeedsBilling(s, d.pickupContact.someoneElse) || d.hasBillingAddress;
  }
  return d.hasShippingAddress && (d.billingSameAsShipping || d.hasBillingAddress);
}

function done(step: CheckoutStep, p: CheckoutProgress): boolean {
  switch (step) {
    case "delivery":
      return p.hasDestination && p.hasShippingMethod;
    case "details":
      return p.detailsComplete;
    case "payment":
      return false;
  }
}

export function firstIncompleteStep(p: CheckoutProgress): CheckoutStep {
  return CHECKOUT_STEPS.find((s) => !done(s, p)) ?? "payment";
}

// The requested step when it's one the shopper has already reached, else the first one still owed - a ?step= can
// reopen a finished step but never skip ahead
export function activeStep(p: CheckoutProgress, requested?: string | null): CheckoutStep {
  const first = firstIncompleteStep(p);
  const want = CHECKOUT_STEPS.find((s) => s === requested);
  if (want && CHECKOUT_STEPS.indexOf(want) <= CHECKOUT_STEPS.indexOf(first)) return want;
  return first;
}

export function stepStatus(
  step: CheckoutStep,
  active: CheckoutStep,
  p: CheckoutProgress,
): StepStatus {
  if (step === active) return "active";
  // Everything before the first owed step is done; past it nothing is, even if a later step happens to hold a value
  const first = CHECKOUT_STEPS.indexOf(firstIncompleteStep(p));
  return CHECKOUT_STEPS.indexOf(step) < first && done(step, p) ? "completed" : "pending";
}

// Business days from today - a carrier's estimate counts working days, so a Friday order quoted 2 days lands Tuesday
export function estimatedArrival(days: number, from: Date = new Date()): Date {
  const d = new Date(from);
  let left = days;
  while (left > 0) {
    d.setDate(d.getDate() + 1);
    const wd = d.getDay();
    if (wd !== 0 && wd !== 6) left--;
  }
  return d;
}
