// What the checkout asks, read off the requirements the backend computes from the store's delivery options - the page,
// the step forms and their schemas all decide from this, so the storefront never re-derives a rule placement enforces

import type { FieldMode } from "./settings";

export type Requirement = {
  scope: string; // "order", or the key of the delivery group it belongs to
  type: string;
  label: string;
  mode: "REQUIRED" | "OPTIONAL";
  status: "SATISFIED" | "MISSING" | "INVALID" | "OPTIONAL";
  code: string;
  message: string;
};

export const ORDER_SCOPE = "order";

// The blocks the delivery step owns - everything a delivery group asks, chosen before the shopper's own details
const DELIVERY_TYPES = new Set(["delivery_option", "point"]);

export function isDeliveryRequirement(r: Requirement): boolean {
  if (r.scope === ORDER_SCOPE) return r.type === "delivery_option"; // a line nothing can deliver
  return DELIVERY_TYPES.has(r.type);
}

// The details step asks who the order is for and where it's billed - collector is per group but asked with the contact
export function isDetailsRequirement(r: Requirement): boolean {
  if (r.type === "collector") return true;
  if (r.scope !== ORDER_SCOPE) return false;
  return ["email", "phone", "shipping_address", "billing_address"].includes(r.type);
}

export const blocking = (r: Requirement) => r.status === "MISSING" || r.status === "INVALID";

// What the details step asks, from the requirements - a delivery to the shopper asks their address, a pickup who
// collects, and billing and phone as strictly as any chosen delivery (or a digital line) does
export type DetailsAsks = {
  shipping: boolean;
  collector: boolean;
  billing: FieldMode;
  phone: FieldMode;
};

function modeOf(reqs: Requirement[], type: string): FieldMode {
  const r = reqs.find((q) => q.scope === ORDER_SCOPE && q.type === type);
  if (!r) return "hidden";
  return r.mode === "REQUIRED" ? "required" : "optional";
}

export function detailsAsks(reqs: Requirement[]): DetailsAsks {
  return {
    shipping: reqs.some((r) => r.scope === ORDER_SCOPE && r.type === "shipping_address"),
    collector: reqs.some((r) => r.type === "collector"),
    billing: modeOf(reqs, "billing_address"),
    phone: modeOf(reqs, "phone"),
  };
}

// The requirements of one delivery group, in the order the group asks them
export function groupRequirements(reqs: Requirement[], groupKey: string): Requirement[] {
  return reqs.filter((r) => r.scope === groupKey);
}
