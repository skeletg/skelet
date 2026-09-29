import type { Metadata } from "next";
import { Link } from "@/app/components/LocaleLink";
import { getLocale, getTranslations } from "next-intl/server";
import { withLocalePrefix } from "@/app/lib/locale/path";
import { notFound, redirect } from "next/navigation";
import { AnalyticsEvent } from "@/app/components/AnalyticsEvent";
import { BackButton } from "@/app/components/BackButton";
import { OrderDetailView } from "@/app/components/OrderDetailView";
import { getResolvedConsent } from "@/app/lib/consent/read";
import { displayName } from "@/app/lib/display";
import { getCustomerOrNull } from "@/app/lib/customer/read";
import { gqlFetch } from "@/app/lib/graphql";
import { GuestOrderDocument } from "@/app/lib/checkout/operations.gql";

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
  }>;
}) {
  const [customer, t] = await Promise.all([
    getCustomerOrNull(),
    getTranslations("order.guest"),
  ]);
  const ta = await getTranslations("account.orders");
  const { id } = await params;
  const sp = await searchParams;

  if (customer) {
    redirect(await withLocalePrefix(`/account/orders/${id}`, await getLocale()));
  }

  const email = (Array.isArray(sp.email) ? sp.email[0] : sp.email) ?? "";
  if (!email) {
    notFound();
  }
  const justPlacedRaw = Array.isArray(sp.just_placed)
    ? sp.just_placed[0]
    : sp.just_placed;
  const justPlaced = justPlacedRaw === "1";

  let order: Awaited<ReturnType<typeof loadOrder>> | null = null;
  try {
    order = await loadOrder(id, email);
  } catch {
    /* fall through to notFound below */
  }
  if (!order) notFound();

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
              value: order.total / 100,
              tax: order.taxAmount / 100,
              shipping: order.shippingAmount / 100,
              items: order.lines.map((line) => ({
                item_id: line.sku,
                item_name: displayName(line.name, line.sku),
                quantity: line.quantity,
                price: line.unitPrice / 100,
              })),
            },
          }}
        />
      )}

      <div className="mb-4">
        <BackButton fallbackHref="/" label={t("continue")} />
      </div>

      {justPlaced && (
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
      <OrderDetailView order={{ ...order, events: [] }} />
    </main>
  );
}

async function loadOrder(id: string, email: string) {
  const res = await gqlFetch(GuestOrderDocument, { id, email });
  return res.data.guestOrder;
}
