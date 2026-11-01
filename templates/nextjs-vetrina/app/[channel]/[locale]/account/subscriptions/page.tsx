import { redirect } from "next/navigation";
import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";

import { withLocalePrefix } from "@skelet/storefront-next/locale/path";
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
import { formatPrice } from "@skelet/storefront-core/format";
import { getCurrentCustomer } from "@skelet/storefront-next/customer/read";
import { loadErrorMessage } from "@skelet/storefront-next/graphql";
import {
  cancelSubscriptionAction,
  setSubscriptionPaymentMethodAction,
} from "@skelet/storefront-next/subscriptions/actions";
import { getMySubscriptions, type Subscription } from "@skelet/storefront-next/subscriptions/read";
import { getMyPaymentMethods } from "@skelet/storefront-next/payment-methods/actions";

export async function generateMetadata({
  params,
}: PageProps<"/[channel]/[locale]/account/subscriptions">): Promise<Metadata> {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "account.subscriptions" });
  return { title: t("title") };
}

function one(v: string | string[] | undefined): string | undefined {
  return Array.isArray(v) ? v[0] : v;
}

export default async function SubscriptionsPage({
  params,
  searchParams,
}: PageProps<"/[channel]/[locale]/account/subscriptions">) {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "account.subscriptions" });
  const tc = await getTranslations({ locale, namespace: "common" });
  const customer = await getCurrentCustomer();
  if (!customer) {
    redirect(await withLocalePrefix("/account/login?returnTo=/account/subscriptions", locale));
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
      t("loadFailed"),
    );
  }

  return (
    <>
      <AccountPageHeader
        title={t("title")}
        description={t("description")}
      />

      {actionError ? (
        <div className="mb-4">
          <FormError>
            {actionError === "cancel" ? t("cancelFailed") : t("cardFailed")}
          </FormError>
        </div>
      ) : actionDone ? (
        <div className="mb-4">
          <FormSuccess>
            {actionDone === "canceled" ? t("canceled") : t("cardUpdated")}
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
          title={t("emptyTitle")}
          description={t("emptyBody")}
          actionHref="/products"
          actionLabel={tc("browseProducts")}
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
                      {t("every", {
                        count: s.intervalCount,
                        unit: s.intervalUnit.toLowerCase(),
                      })}
                      {active
                        ? ` · ${t("nextBilling", {
                            date: new Date(s.nextBillingAt).toLocaleDateString(),
                          })}`
                        : null}
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
                            {t("billingCard")}
                          </span>
                          <select
                            name="paymentMethodId"
                            defaultValue={s.paymentMethodId ?? ""}
                            className={`${inputClass} w-56`}
                          >
                            <option value="">
                              {t("noCard")}
                            </option>
                            {savedMethods.map((m) => (
                              <option key={m.id} value={m.id}>
                                {m.brand || tc("card")} •••• {m.last4}
                              </option>
                            ))}
                          </select>
                        </label>
                        <SubmitButton
                          className="inline-flex h-10 items-center rounded-md border border-border bg-background px-4 text-sm font-semibold transition-colors hover:bg-accent"
                          pendingLabel={tc("saving")}
                        >
                          {tc("save")}
                        </SubmitButton>
                      </form>
                    ) : (
                      <p className="max-w-sm text-xs text-muted-foreground">
                        {t("saveCardHint")}
                      </p>
                    )}
                    <form action={cancelSubscriptionAction}>
                      <input type="hidden" name="id" value={s.id} />
                      <SubmitButton
                        className="inline-flex h-10 items-center rounded-md px-4 text-sm font-semibold text-red-600 transition-colors hover:bg-red-50"
                        confirmLabel={t("confirmCancel")}
                        cancelLabel={t("keepIt")}
                        pendingLabel={t("canceling")}
                      >
                        {t("cancel")}
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
