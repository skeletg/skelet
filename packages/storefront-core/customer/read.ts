// Customer profile reader


import { perRequest } from "../request";
import { getAccessToken } from "../auth/session";
import { MyProfileDocument } from "./operations.gql";
import { gqlFetch, isAuthError } from "../graphql";

export type Customer = {
  id: string;
  email: string;
  firstName: string;
  lastName: string;
  phone: string | null;
  isEmailVerified: boolean;
  storeCreditBalance: number;
  defaultBillingAddress: Address | null;
  defaultShippingAddress: Address | null;
  addresses: Address[];
};

export type Address = {
  id: string;
  firstName: string;
  lastName: string;
  company: string | null;
  street1: string;
  street2: string | null;
  city: string;
  state: string | null;
  postalCode: string;
  countryCode: string;
  phone: string | null;
};

// Every account page reads a null as "not signed in" and bounces to login, so a backend
// hiccup must not log people out - signed-out and broken are different answers
export type CustomerLoad =
  | { kind: "customer"; customer: Customer }
  | { kind: "anonymous" }
  | { kind: "error"; error: unknown };

export const loadCurrentCustomer = perRequest(async (): Promise<CustomerLoad> => {
  const token = await getAccessToken();
  if (!token) return { kind: "anonymous" };
  try {
    const res = await gqlFetch(MyProfileDocument);
    return { kind: "customer", customer: res.data.myProfile };
  } catch (err) {
    // A dead / rejected session really is signed out; anything else is our fault
    if (isAuthError(err)) return { kind: "anonymous" };
    return { kind: "error", error: err };
  }
});

// Throws on a backend failure so the route's error boundary renders instead of the
// caller silently redirecting a still-signed-in customer to the login page
export async function getCurrentCustomer(): Promise<Customer | null> {
  const res = await loadCurrentCustomer();
  if (res.kind === "error") throw res.error;
  return res.kind === "customer" ? res.customer : null;
}

// For chrome and public pages that only use the customer to decorate themselves - those
// must still render when the profile read fails, so they take the tolerant answer
export async function getCustomerOrNull(): Promise<Customer | null> {
  const res = await loadCurrentCustomer();
  return res.kind === "customer" ? res.customer : null;
}
