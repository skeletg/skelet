import type { Metadata } from "next";
import { Link } from "@/app/components/LocaleLink";
import { getLocale, getTranslations } from "next-intl/server";
import { withLocalePrefix } from "@skelet/storefront-next/locale/path";
import { notFound, redirect } from "next/navigation";
import { AnalyticsEvent } from "@/app/components/AnalyticsEvent";
import { BackButton } from "@/app/components/BackButton";
import { OrderDetailView } from "@/app/components/OrderDetailView";
import { getResolvedConsent } from "@skelet/storefront-next/consent/read";
import { displayName } from "@skelet/storefront-core/display";
import { getCustomerOrNull } from "@skelet/storefront-next/customer/read";
import { gqlFetch } from "@skelet/storefront-next/graphql";
import { GuestOrderDocument } from "@skelet/storefront-core/checkout/operations.gql";
import { SubmitButton } from "@/app/components/account/SubmitButton";
import { resendVerificationEmailAction } from "@skelet/storefront-next/auth/actions";
import { toMajor } from "@skelet/storefront-core/format";

export async function generateMetadata({
  params,
}: PageProps<"/[channel]/[locale]/orders/[id]">): Promise<Metadata> {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "account.orders" });
  return { title: t("detailTitle") };
}

export default async function GuestOrderPage({
  params,
  searchParams,
}: {
  params: Promise<{ id: string }>;
  searchParams: Promise<{
    email?: string | string[];
    just_placed?: string | string[];
    payment?: string | string[];
    resent?: string | string[];
  }>;
}) {
  const [customer, t] = await Promise.all([
    getCustomerOrNull(),
    getTranslations("order.guest"),
  ]);
  const ta = await getTranslations("account.orders");
  const tc = await getTranslations("common");
  const { id } = await params;
  const sp = await searchParams;

  const email = (Array.isArray(sp.email) ? sp.email[0] : sp.email) ?? "";
  const accountOrderPath = await withLocalePrefix(
    `/account/orders/${id}`,
    await getLocale(),
  );
  if (!email) {
    if (customer) redirect(accountOrderPath);
    notFound();
  }
  const justPlacedRaw = Array.isArray(sp.just_placed)
    ? sp.just_placed[0]
    : sp.just_placed;
  const justPlaced = justPlacedRaw === "1";
  const paymentIncomplete = (Array.isArray(sp.payment) ? sp.payment[0] : sp.payment) === "incomplete";

  let order: Awaited<ReturnType<typeof loadOrder>> | null = null;
  try {
    order = await loadOrder(id, email);
  } catch {
    /* fall through to notFound below */
  }
  // guestOrder only answers while the order is unclaimed. A signed-in shopper can still be looking at one - an account
  // takes over its email's guest orders only once the address is verified - so they only move to the account page
  // when the order is no longer a guest one
  if (!order) {
    if (customer) redirect(accountOrderPath);
    notFound();
  }
  const awaitingClaim =
    customer !== null &&
    !customer.isEmailVerified &&
    customer.email.toLowerCase() === email.toLowerCase();
  const resent = Array.isArray(sp.resent) ? sp.resent[0] : sp.resent;

  const consent = justPlaced ? await getResolvedConsent() : null;

  return (
    <main className="mx-auto w-full max-w-3xl px-4 pt-4 pb-16 sm:px-6">
      {justPlaced && consent && (
        <AnalyticsEvent
          enabled={consent.marketing}
          event="purchase"
          stripParam="just_placed"
          payload={{
            ecommerce: {
              transaction_id: order.id,
              currency: order.currencyCode,
              value: toMajor(order.total, order.currencyCode),
              tax: toMajor(order.taxAmount, order.currencyCode),
              shipping: toMajor(order.shippingAmount, order.currencyCode),
              items: order.lines.map((line) => ({
                item_id: line.sku,
                item_name: displayName(line.name, line.sku),
                quantity: line.quantity,
                price: toMajor(line.unitPrice, order.currencyCode),
              })),
            },
          }}
        />
      )}

      <div className="mb-4">
        <BackButton fallbackHref="/" label={t("continue")} />
      </div>

      {awaitingClaim && (
        <div className="mb-6 rounded-xl border border-amber-300 bg-amber-50 p-5 text-sm text-amber-900">
          <p className="font-semibold">{t("claimTitle")}</p>
          <p className="mt-1">{t("claimBody")}</p>
          {resent === "1" ? (
            <p className="mt-3 font-semibold">{t("claimSent")}</p>
          ) : (
            <form action={resendVerificationEmailAction} className="mt-3">
              <input
                type="hidden"
                name="returnTo"
                value={`/orders/${order.id}?email=${encodeURIComponent(email)}`}
              />
              <SubmitButton
                className="inline-flex h-9 items-center rounded-md border border-amber-400 px-4 text-xs font-semibold hover:bg-amber-100"
                pendingLabel={tc("sending")}
              >
                {resent === "0" ? t("claimResendFailed") : t("claimResend")}
              </SubmitButton>
            </form>
          )}
        </div>
      )}

      {justPlaced && !customer && (
        <section className="glass mb-6 rounded-xl p-5">
          <h2 className="text-lg font-semibold">{ta("thanksTitle")}</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {t.rich("receiptSent", {
              email: () => <span className="font-mono">{email}</span>,
            })}
          </p>
          <Link
            href={`/account/register?returnTo=${encodeURIComponent(
              `/orders/${order.id}?email=${encodeURIComponent(email)}`,
            )}`}
            className="mt-3 inline-flex h-10 items-center rounded-md bg-primary px-5 text-sm font-semibold text-primary-foreground"
          >
            {t("createAccount")}
          </Link>
          <p className="mt-2 text-xs text-muted-foreground">
            {t("oneAccount")}
          </p>
        </section>
      )}

      {/* Admin-facing history - signed-in page only, never the guest receipt */}
      <OrderDetailView order={{ ...order, events: [] }} paymentIncomplete={paymentIncomplete} />
    </main>
  );
}

async function loadOrder(id: string, email: string) {
  const res = await gqlFetch(GuestOrderDocument, { id, email });
  return res.data.guestOrder;
}
