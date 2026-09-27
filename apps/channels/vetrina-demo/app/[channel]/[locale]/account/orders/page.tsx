import { Link } from "@/app/components/LocaleLink";

import { withLocalePrefix } from "@/app/lib/locale/path";
import { redirect } from "next/navigation";
import { ChevronRightIcon, PackageIcon } from "lucide-react";

import {
  AccountCard,
  AccountPageHeader,
  EmptyState,
} from "@/app/components/account/AccountUI";
import { PaymentChip, StatusChip, humanizeStatus } from "@/app/components/account/StatusChip";
import { formatPrice } from "@/app/lib/format";
import { getCurrentCustomer } from "@/app/lib/customer/read";
import { getChannel } from "@/app/lib/channel/read";
import { gqlFetch, loadErrorMessage } from "@/app/lib/graphql";
import { MyOrdersDocument } from "@/app/lib/checkout/operations.gql";

export const metadata = { title: "Your orders" };

export default async function OrdersPage({ params }: PageProps<"/[channel]/[locale]/account/orders">) {
  // Locale straight off the segment - a page taking no props has no request scope for
  // getLocale() to read, so it silently answered with the default language
  const { locale } = await params;

  const [customer, channel] = await Promise.all([
    getCurrentCustomer(),
    getChannel(),
  ]);
  if (!customer) {
    redirect(await withLocalePrefix("/account/login?returnTo=/account/orders", locale));
  }
  const fallbackCurrency = channel?.currencyCode ?? "USD";

  let orders: Array<{
    id: string;
    number: string;
    status: string;
    paymentStatus: string;
    total: number;
    currencyCode: string;
    createdAt: string;
  }> = [];
  let loadError: string | null = null;
  try {
    const res = await gqlFetch(MyOrdersDocument);
    orders = res.data.myOrders;
  } catch (err) {
    loadError = loadErrorMessage(
      "account/orders load failed",
      err,
      "We couldn't load your orders just now. Please try again in a moment.",
    );
  }

  return (
    <>
      <AccountPageHeader
        title="Your orders"
        description={
          orders.length > 0
            ? `${orders.length} order${orders.length === 1 ? "" : "s"} placed with us.`
            : undefined
        }
      />

      {loadError ? (
        <p
          role="alert"
          className="rounded-xl border border-red-300 bg-red-50 p-5 text-sm text-red-700"
        >
          {loadError}
        </p>
      ) : orders.length === 0 ? (
        <EmptyState
          icon={<PackageIcon className="size-5" aria-hidden="true" />}
          title="No orders yet"
          description="Once you place an order it shows up here with tracking, receipts and one-click reordering."
          actionHref="/products"
          actionLabel="Start shopping"
        />
      ) : (
        <AccountCard padded={false}>
          <ul className="divide-y divide-border">
            {orders.map((order) => (
              <li key={order.id}>
                <Link
                  href={`/account/orders/${order.id}`}
                  className="flex items-center gap-4 px-5 py-4 transition-colors hover:bg-accent"
                >
                  <div className="min-w-0 flex-1">
                    <p className="font-mono text-sm font-semibold">
                      {order.number}
                    </p>
                    <div className="mt-1.5 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                      <span>
                        {new Date(order.createdAt).toLocaleDateString()}
                      </span>
                      <StatusChip>{humanizeStatus(order.status)}</StatusChip>
                      <PaymentChip status={order.paymentStatus} />
                    </div>
                  </div>
                  <span className="shrink-0 text-sm font-semibold tabular-nums">
                    {formatPrice(
                      order.total,
                      order.currencyCode || fallbackCurrency,
                    )}
                  </span>
                  <ChevronRightIcon
                    className="size-4 shrink-0 text-muted-foreground"
                    aria-hidden="true"
                  />
                </Link>
              </li>
            ))}
          </ul>
        </AccountCard>
      )}
    </>
  );
}
