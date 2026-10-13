// Checkout reads - the cart's saved checkout, and what its delivery groups are offered. All server-side: the page
// re-renders from these after every step instead of the browser fetching options itself

import { cache } from "react";

import {
  CheckoutStateDocument,
  DeliveryPointsDocument,
  PaymentMethodsDocument,
  type CheckoutStateQuery,
  type DeliveryPointsQuery,
} from "@/app/lib/checkout/operations.gql";
import { gqlFetch } from "@/app/lib/graphql";

import type { Requirement } from "./asks";

export type CheckoutState = Omit<NonNullable<CheckoutStateQuery["cart"]>["checkout"], "requirements"> & {
  requirements: Requirement[];
};
export type CheckoutAddress = NonNullable<CheckoutState["shippingAddress"]>;
export type CheckoutDestination = NonNullable<CheckoutState["destination"]>;
export type DeliveryGroup = CheckoutState["deliveryGroups"][number];
export type DeliveryChoice = DeliveryGroup["choices"][number];
export type DeliveryPoint = DeliveryPointsQuery["deliveryPoints"][number];
export type PaymentMethodOption = {
  providerSlug: string;
  displayName: string;
  sortOrder: number;
  instructions: string;
};

// Throws on a backend failure - the page can't render a checkout it can't read, and the error boundary says so
export const getCheckoutState = cache(async (cartId: string): Promise<CheckoutState | null> => {
  const res = await gqlFetch(CheckoutStateDocument, { id: cartId });
  return (res.data.cart?.checkout as CheckoutState | undefined) ?? null;
});

export async function getPaymentMethods(countryCode: string): Promise<PaymentMethodOption[]> {
  try {
    const res = await gqlFetch(PaymentMethodsDocument, { countryCode });
    return res.data.paymentMethods.slice().sort((a, b) => a.sortOrder - b.sortOrder);
  } catch {
    return [];
  }
}

// The stores or PO boxes a group's chosen delivery can go to, near the cart's destination - an unreachable
// source is reported separately from an empty result so the shopper can retry
export type DeliveryPointsResult = { points: DeliveryPoint[]; unavailable: boolean };

export async function getDeliveryPoints(cartId: string, groupKey: string, search?: string): Promise<DeliveryPointsResult> {
  try {
    const res = await gqlFetch(DeliveryPointsDocument, { cartId, groupKey, search: search || null });
    return { points: res.data.deliveryPoints, unavailable: false };
  } catch {
    return { points: [], unavailable: true };
  }
}
