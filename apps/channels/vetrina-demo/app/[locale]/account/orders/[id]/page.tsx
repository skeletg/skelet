import { notFound, redirect } from "next/navigation";
import { getLocale } from "next-intl/server";

import { withLocalePrefix } from "@/app/lib/locale/path";
import { AnalyticsEvent } from "@/app/components/AnalyticsEvent";
import { BackButton } from "@/app/components/BackButton";
import { OrderDetailView } from "@/app/components/OrderDetailView";
import { getResolvedConsent } from "@/app/lib/consent/read";
import { displayName } from "@/app/lib/display";
import { formatPrice } from "@/app/lib/format";
import { getCurrentCustomer } from "@/app/lib/customer/read";
import { gqlFetch, isBackendFailure } from "@/app/lib/graphql";
import { MyOrderDocument } from "@/app/lib/checkout/operations.gql";
import { ReorderButton } from "@/app/[locale]/account/orders/ReorderButton";
import { ReturnRequestForm, type ReturnableItem } from "@/app/[locale]/account/orders/ReturnRequestForm";

export const metadata = { title: "Order" };

export default async function OrderDetailPage({
  params,
  searchParams,
}: {
  params: Promise<{ id: string }>;
  searchParams: Promise<{ just_placed?: string | string[] }>;
}) {
  const customer = await getCurrentCustomer();
  if (!customer) {
    redirect(await withLocalePrefix("/account/login", await getLocale()));
  }
  const { id } = await params;
  const sp = await searchParams;
  const justPlacedRaw = Array.isArray(sp.just_placed)
    ? sp.just_placed[0]
    : sp.just_placed;
  const justPlaced = justPlacedRaw === "1";

  let order: Awaited<ReturnType<typeof loadOrder>>;
  try {
    order = await loadOrder(id);
  } catch (err) {
    // "We can't reach the store" is not "this order doesn't exist" - let the error
    // boundary say so instead of showing a 404 for a live order
    if (isBackendFailure(err)) throw err;
    notFound();
  }

  const consent = justPlaced ? await getResolvedConsent() : null;

  const returnableByLine = new Map(
    order.returnableLines.map((rl) => [rl.orderLineId, rl.quantity]),
  );
  const returnableItems: ReturnableItem[] = order.lines
    .filter((l) => (returnableByLine.get(l.id) ?? 0) > 0)
    .map((l) => ({
      orderLineId: l.id,
      productName: displayName(l.productName, l.productSku),
      productSku: l.productSku,
      returnableQty: returnableByLine.get(l.id) ?? 0,
    }));

  return (
    <div className="max-w-3xl">
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
                item_id: line.productSku,
                item_name: displayName(line.productName, line.productSku),
                quantity: line.quantity,
                price: line.unitPrice / 100,
              })),
            },
          }}
        />
      )}

      <div className="mb-4">
        <BackButton fallbackHref="/account/orders" label="All orders" />
      </div>

      {justPlaced ? (
        <section className="glass mb-6 rounded-xl p-5">
          <h2 className="text-lg font-semibold">Thanks for your order</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            A confirmation is on its way to your inbox. You can track everything
            from this page.
          </p>
        </section>
      ) : null}

      <OrderDetailView
        order={order}
        headerAction={<ReorderButton orderId={order.id} />}
        extraSections={
          <>
            {order.returns.length > 0 ? (
              <section className="glass mt-6 rounded-xl p-5">
                <h2 className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                  Returns
                </h2>
                <ul className="mt-3 divide-y divide-border">
                  {order.returns.map((ret) => (
                    <li key={ret.id} className="py-3 text-sm first:pt-0">
                      <div className="flex items-center justify-between gap-2">
                        <span className="font-mono font-semibold">
                          {ret.rmaNumber}
                        </span>
                        <span className="rounded-sm bg-muted px-2.5 py-0.5 text-[0.65rem] font-semibold uppercase tracking-wider text-muted-foreground">
                          {ret.status}
                        </span>
                      </div>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {ret.lines.reduce((n, l) => n + l.quantity, 0)} item(s)
                        {ret.refundAmount > 0
                          ? ` · refunded ${formatPrice(ret.refundAmount, order.currencyCode)}`
                          : ""}
                      </p>
                    </li>
                  ))}
                </ul>
              </section>
            ) : null}

            {returnableItems.length > 0 ? (
              <section className="glass mt-6 rounded-xl p-5">
                <h2 className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                  Request a return
                </h2>
                <p className="mt-1 text-xs text-muted-foreground">
                  Choose how many of each item to return. We&rsquo;ll review your
                  request and email you.
                </p>
                <div className="mt-3">
                  <ReturnRequestForm
                    orderId={order.id}
                    items={returnableItems}
                  />
                </div>
              </section>
            ) : null}
          </>
        }
      />
    </div>
  );
}

async function loadOrder(id: string) {
  const res = await gqlFetch(MyOrderDocument, { id });
  return res.data.myOrder;
}
