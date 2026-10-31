// Subscription readers. The account page wants the failure surfaced, the payment-methods
// page only wants to know which saved cards a renewal depends on

import { MySubscriptionsDocument } from "@/app/lib/customer/operations.gql";
import { gqlFetch } from "@/app/lib/graphql";

export type Subscription = {
  id: string;
  productId: string;
  title: string;
  status: string;
  intervalUnit: string;
  intervalCount: number;
  price: number;
  currencyCode: string;
  currentPeriodEnd: string;
  nextBillingAt: string;
  canceledAt: string | null;
  createdAt: string;
  paymentMethodId: string | null;
};

export async function getMySubscriptions(): Promise<Subscription[]> {
  const res = await gqlFetch(MySubscriptionsDocument);
  return res.data.mySubscriptions;
}

// Maps a saved card id to the titles of the active subscriptions billing against it, so
// "remove card" can say what it would break. Tolerant - a miss just drops the warning
export async function getActiveSubscriptionsByCard(): Promise<
  Map<string, string[]>
> {
  const byCard = new Map<string, string[]>();
  let subs: Subscription[];
  try {
    subs = await getMySubscriptions();
  } catch {
    return byCard;
  }
  for (const s of subs) {
    if (s.status !== "ACTIVE" || !s.paymentMethodId) continue;
    const titles = byCard.get(s.paymentMethodId) ?? [];
    titles.push(s.title);
    byCard.set(s.paymentMethodId, titles);
  }
  return byCard;
}
