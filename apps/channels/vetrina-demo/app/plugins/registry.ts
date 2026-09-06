// Storefront extension points, so a plugin's UI lives in app/plugins/<slug>/ rather than threaded
// through the pages it appears on
//
// Deliberately smaller than the dashboard's: routes need no registration here because Next's
// filesystem router IS the glob - a plugin's page lands at its real path and simply exists. What is
// left is the handful of places a plugin needs to appear INSIDE a core page, which is what these two
// registries cover
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

const navItems: AccountNavItem[] = [];
const slots: Slot[] = [];

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

// Between-scenario reset - registration is module-level side effect, so tests need a way back
export function __resetForTests() {
  navItems.length = 0;
  slots.length = 0;
}
