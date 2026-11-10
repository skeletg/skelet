// Storefront extension points, so a plugin's UI lives in app/plugins/<slug>/ rather than threaded
// through the pages it appears on
//
// Deliberately smaller than the dashboard's: routes need no registration here because Next's
// filesystem router IS the glob - a plugin's page lands at its real path and simply exists. What is
// left is the handful of places a plugin needs to appear INSIDE a core page, which is what these
// registries cover - plus a payment gateway's card fields, which checkout drives through a handle
//
// Everything here is client-safe on purpose. The three host components (AccountNav, CartView and the
// cart action row) are all "use client", so a server-rendered slot could not mount in them. A plugin
// that needs server data calls its own "use server" action from its slot component, which is what
// b2b does - core never fetches on a plugin's behalf, and never learns why a slot is or is not shown

import type { ComponentType, ReactNode } from "react";

// A link in the account sidebar. DATA rather than a rendered node, so a plugin's entry gets the same
// active state, icon sizing and strip/sidebar variants as the built-in ones instead of reimplementing
// them and drifting
export type AccountNavItem = {
  id: string; // conventionally "<plugin>:<thing>" - duplicates throw at registration
  href: string;
  label: string;
  Icon: ComponentType<{ className?: string }>;
  order?: number; // lower renders earlier, default 100 - core sits at 10-50
  // Whether this account has the item at all - b2b's Company link is for company members only, and
  // only the plugin can answer that. A HOOK because the answer needs a fetch and the host is a
  // client component; AccountNav renders each item through its own component so the call is
  // unconditional per instance and the rules of hooks hold
  useVisible?: () => boolean;
};

// Anywhere else a plugin renders into a core page. Zone is free-form, conventionally
// "<surface>:<target>", so a new one needs no change here - today "cart:summary" (under the totals)
// and "cart:actions" (the row beside Proceed to checkout), both with ctx {subtotal, currency}
//
// ctx is what the host already has in hand and the slot would otherwise have to re-derive - the cart
// subtotal being the case in point. Typed per zone by convention rather than by the registry, which
// would need a union that every new zone edits
export type Slot<TCtx = unknown> = {
  id: string;
  zone: string;
  order?: number;
  render: (ctx: TCtx) => ReactNode;
};

// A gateway's card fields at checkout. While its provider is picked, the payment step renders Fields, which hands the
// step a handle through register: tokenize runs right before the order is placed, and challenge after it when the
// gateway parks on an in-page step (3DS) - the backend then asks the gateway itself whether the payment went through.
// Pay stays disabled until Fields registers
export type PaymentHandle = {
  // The single-use token placeOrder takes, or null when the gateway confirms after placing instead
  tokenize: () => Promise<string | null>;
  challenge?: (clientSecret: string) => Promise<void>;
};

export type PaymentFieldsProps = {
  amountCents: number;
  currency: string;
  // Call it from an effect once the fields are ready, never while rendering, and with null when they unmount
  register: (handle: PaymentHandle | null) => void;
};

export type PaymentUI = {
  provider: string; // the payment provider slug it pays for - one UI each
  Fields: ComponentType<PaymentFieldsProps>;
  // Runs the in-page step for a saved card, where no Fields are mounted to hand over a handle
  challenge?: (clientSecret: string) => Promise<void>;
};

const navItems: AccountNavItem[] = [];
const slots: Slot[] = [];
const paymentUIs = new Map<string, PaymentUI>();

export function registerAccountNavItem(item: AccountNavItem) {
  if (navItems.some((i) => i.id === item.id)) {
    throw new Error(`plugins: duplicate account nav item "${item.id}"`);
  }
  navItems.push(item);
}

export function getAccountNavItems(): AccountNavItem[] {
  return [...navItems].sort((a, b) => (a.order ?? 100) - (b.order ?? 100));
}

export function registerSlot<TCtx = unknown>(slot: Slot<TCtx>) {
  if (slots.some((s) => s.id === slot.id && s.zone === slot.zone)) {
    throw new Error(`plugins: duplicate slot "${slot.id}" in zone "${slot.zone}"`);
  }
  slots.push(slot as Slot);
}

export function getSlots<TCtx = unknown>(zone: string): Slot<TCtx>[] {
  return slots
    .filter((s) => s.zone === zone)
    .sort((a, b) => (a.order ?? 100) - (b.order ?? 100)) as Slot<TCtx>[];
}

export function registerPaymentUI(ui: PaymentUI) {
  if (paymentUIs.has(ui.provider)) {
    throw new Error(`plugins: duplicate payment UI for "${ui.provider}"`);
  }
  paymentUIs.set(ui.provider, ui);
}

export function getPaymentUI(provider: string): PaymentUI | undefined {
  return paymentUIs.get(provider);
}

// Between-scenario reset - registration is module-level side effect, so tests need a way back
export function __resetForTests() {
  navItems.length = 0;
  slots.length = 0;
  paymentUIs.clear();
}
