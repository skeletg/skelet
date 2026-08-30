"use server";

import { loadCurrentCustomer, type Customer } from "@/app/lib/customer/read";

// Fetched after hydration - a server-side auth read forces the route dynamic, and only the header chrome's fields are returned, never the full profile
export type HeaderIdentity = { id: string; firstName: string; lastName: string; email: string } | null;

export async function loadHeaderIdentity(): Promise<HeaderIdentity> {
  const res = await loadCurrentCustomer();
  if (res.kind !== "customer") return null;
  const c: Customer = res.customer;
  return { id: c.id, firstName: c.firstName, lastName: c.lastName, email: c.email };
}
