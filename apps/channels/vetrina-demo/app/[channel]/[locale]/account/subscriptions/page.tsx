import { redirect } from "next/navigation";
import { getLocale } from "next-intl/server";

import { withLocalePrefix } from "@/app/lib/locale/path";
import { RefreshCwIcon } from "lucide-react";

import {
  AccountPageHeader,
  EmptyState,
  FormError,
  FormSuccess,
  inputClass,
} from "@/app/components/account/AccountUI";
import { StatusChip, humanizeStatus } from "@/app/components/account/StatusChip";
import { SubmitButton } from "@/app/components/account/SubmitButton";
import { formatPrice } from "@/app/lib/format";
import { getCurrentCustomer } from "@/app/lib/customer/read";
import { loadErrorMessage } from "@/app/lib/graphql";
import {
  cancelSubscriptionAction,
  setSubscriptionPaymentMethodAction,
} from "@/app/lib/subscriptions/actions";
import { getMySubscriptions, type Subscription } from "@/app/lib/subscriptions/read";
import { getMyPaymentMethods } from "@/app/lib/payment-methods/actions";

export const metadata = { title: "Subscriptions" };

function intervalLabel(count: number, unit: string): string {
  const u = unit.toLowerCase();
  return count === 1 ? `every ${u}` : `every ${count} ${u}s`;
}

function one(v: string | string[] | undefined): string | undefined {
  return Array.isArray(v) ? v[0] : v;
}

export default async function SubscriptionsPage({
  searchParams,
}: {
  searchParams: Promise<{
    error?: string | string[];
    done?: string | string[];
  }>;
}) {
  const customer = await getCurrentCustomer();
  if (!customer) {
    redirect(await withLocalePrefix("/account/login?returnTo=/account/subscriptions", await getLocale()));
  }

  const sp = await searchParams;
  const actionError = one(sp.error);
  const actionDone = one(sp.done);

  const savedMethods = await getMyPaymentMethods();
  let subscriptions: Subscription[] = [];
  let loadError: string | null = null;
  try {
    subscriptions = await getMySubscriptions();
  } catch (err) {
    loadError = loadErrorMessage(
      "account/subscriptions load failed",
      err,
      "We couldn't load your subscriptions just now. Please try again in a moment.",
    );
  }

  return (
    <>
      <AccountPageHeader
        title="Subscriptions"
        description="Recurring deliveries bill automatically. Cancel any time - it stops the next charge."
      />

      {actionError ? (
        <div className="mb-4">
          <FormError>
            {actionError === "cancel"
              ? "We couldn't cancel that subscription - it's still active. Please try again."
              : "We couldn't change the billing card. Please try again."}
          </FormError>
        </div>
      ) : actionDone ? (
        <div className="mb-4">
          <FormSuccess>
            {actionDone === "canceled"
              ? "Subscription canceled - you won't be charged again."
              : "Billing card updated."}
          </FormSuccess>
        </div>
      ) : null}

      {loadError ? (
        <p
          role="alert"
          className="rounded-xl border border-red-300 bg-red-50 p-5 text-sm text-red-700"
        >
          {loadError}
        </p>
      ) : subscriptions.length === 0 ? (
        <EmptyState
          icon={<RefreshCwIcon className="size-5" aria-hidden="true" />}
          title="No subscriptions"
          description="Products you buy on a repeating schedule show up here, with their next billing date."
          actionHref="/products"
          actionLabel="Browse products"
        />
      ) : (
        <ul className="space-y-4">
          {subscriptions.map((s) => {
            const active = s.status === "ACTIVE";
            return (
              <li key={s.id} className="glass rounded-xl p-5">
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div className="min-w-0">
                    <p className="font-semibold">{s.title}</p>
                    <p className="mt-1 text-sm text-muted-foreground">
                      {formatPrice(s.price, s.currencyCode)}{" "}
                      {intervalLabel(s.intervalCount, s.intervalUnit)}
                      {active ? (
                        <>
                          {" · next billing "}
                          {new Date(s.nextBillingAt).toLocaleDateString()}
                        </>
                      ) : null}
                    </p>
                  </div>
                  <StatusChip tone={active ? "positive" : "muted"}>
                    {humanizeStatus(s.status)}
                  </StatusChip>
                </div>

                {active && (
                  <div className="mt-4 flex flex-wrap items-end justify-between gap-3 border-t border-border pt-4">
                    {savedMethods.length > 0 ? (
                      <form
                        action={setSubscriptionPaymentMethodAction}
                        className="flex flex-wrap items-end gap-2"
                      >
                        <input
                          type="hidden"
                          name="subscriptionId"
                          value={s.id}
                        />
                        <label className="flex flex-col gap-1.5 text-xs">
                          <span className="font-medium text-muted-foreground">
                            Billing card
                          </span>
                          <select
                            name="paymentMethodId"
                            defaultValue={s.paymentMethodId ?? ""}
                            className={`${inputClass} w-56`}
                          >
                            <option value="">
                              No card - renewals will fail
                            </option>
                            {savedMethods.map((m) => (
                              <option key={m.id} value={m.id}>
                                {m.brand || "Card"} •••• {m.last4}
                              </option>
                            ))}
                          </select>
                        </label>
                        <SubmitButton
                          className="inline-flex h-10 items-center rounded-md border border-border bg-background px-4 text-sm font-semibold transition-colors hover:bg-accent"
                          pendingLabel="Saving…"
                        >
                          Save
                        </SubmitButton>
                      </form>
                    ) : (
                      <p className="max-w-sm text-xs text-muted-foreground">
                        Save a card at checkout to enable automatic renewals.
                      </p>
                    )}
                    <form action={cancelSubscriptionAction}>
                      <input type="hidden" name="id" value={s.id} />
                      <SubmitButton
                        className="inline-flex h-10 items-center rounded-md px-4 text-sm font-semibold text-red-600 transition-colors hover:bg-red-50"
                        confirmLabel="Yes, cancel it"
                        cancelLabel="Keep it"
                        pendingLabel="Canceling…"
                      >
                        Cancel subscription
                      </SubmitButton>
                    </form>
                  </div>
                )}
              </li>
            );
          })}
        </ul>
      )}
    </>
  );
}
