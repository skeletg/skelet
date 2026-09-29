import { redirect } from "next/navigation";
import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";

import { withLocalePrefix } from "@/app/lib/locale/path";
import { CreditCardIcon } from "lucide-react";

import {
  AccountCard,
  AccountPageHeader,
  EmptyState,
  FormError,
} from "@/app/components/account/AccountUI";
import { SubmitButton } from "@/app/components/account/SubmitButton";
import { getCurrentCustomer } from "@/app/lib/customer/read";
import {
  deletePaymentMethodAction,
  getMyPaymentMethods,
} from "@/app/lib/payment-methods/actions";
import { getActiveSubscriptionsByCard } from "@/app/lib/subscriptions/read";

export async function generateMetadata({
  params,
}: PageProps<"/[channel]/[locale]/account/payment-methods">): Promise<Metadata> {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "account.paymentMethods" });
  return { title: t("title") };
}

export default async function PaymentMethodsPage({
  params,
  searchParams,
}: PageProps<"/[channel]/[locale]/account/payment-methods">) {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "account.paymentMethods" });
  const tc = await getTranslations({ locale, namespace: "common" });
  const customer = await getCurrentCustomer();
  if (!customer) redirect(await withLocalePrefix("/account/login?returnTo=/account/payment-methods", locale));
  const [methods, subsByCard] = await Promise.all([
    getMyPaymentMethods(),
    getActiveSubscriptionsByCard(),
  ]);

  const sp = await searchParams;
  const actionError = Array.isArray(sp.error) ? sp.error[0] : sp.error;

  return (
    <div className="max-w-2xl">
      <AccountPageHeader
        title={t("title")}
        description={t("description")}
      />

      {actionError ? (
        <div className="mb-4">
          <FormError>{t("removeFailed")}</FormError>
        </div>
      ) : null}

      {methods.length === 0 ? (
        <EmptyState
          icon={<CreditCardIcon className="size-5" aria-hidden="true" />}
          title={t("emptyTitle")}
          description={t("emptyBody")}
        />
      ) : (
        <AccountCard padded={false}>
          <ul className="divide-y divide-border">
            {methods.map((m) => {
              // Removing the card a renewal bills against silently breaks that renewal
              const boundSubs = subsByCard.get(m.id) ?? [];
              return (
                <li
                  key={m.id}
                  className="flex items-center justify-between gap-4 px-5 py-4"
                >
                  <div className="flex items-center gap-3">
                    <span className="flex size-10 items-center justify-center rounded-full bg-muted">
                      <CreditCardIcon className="size-4" aria-hidden="true" />
                    </span>
                    <div>
                      <p className="text-sm font-semibold">
                        {m.brand || tc("card")} •••• {m.last4 || "????"}
                      </p>
                      <p className="text-xs text-muted-foreground">
                        {m.expMonth > 0
                          ? t("expires", {
                              month: String(m.expMonth).padStart(2, "0"),
                              year: m.expYear,
                            })
                          : m.providerSlug}
                      </p>
                      {boundSubs.length > 0 && (
                        <p className="mt-1 text-xs font-medium text-amber-700">
                          {t("boundSubs", { subs: boundSubs.join(", ") })}
                        </p>
                      )}
                    </div>
                  </div>
                  <form action={deletePaymentMethodAction}>
                    <input type="hidden" name="id" value={m.id} />
                    <SubmitButton
                      className="rounded-md px-3 py-1.5 text-sm font-semibold text-red-600 transition-colors hover:bg-red-50"
                      confirmLabel={
                        boundSubs.length > 0 ? t("removeAnyway") : t("confirmRemove")
                      }
                      pendingLabel={t("removing")}
                    >
                      {t("remove")}
                    </SubmitButton>
                  </form>
                </li>
              );
            })}
          </ul>
        </AccountCard>
      )}
    </div>
  );
}
