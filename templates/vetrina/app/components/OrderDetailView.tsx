import type { ReactNode } from "react";
import { PackageIcon, TruckIcon } from "lucide-react";
import { useTranslations } from "next-intl";

import type { OrderSummaryFieldsFragment } from "@skelet/storefront-next/checkout/operations.gql";
import { displayName } from "@skelet/storefront-next/display";
import { formatPrice } from "@skelet/storefront-next/format";
import {
  PaymentChip,
  StatusChip,
  humanizeStatus,
} from "./account/StatusChip";
import { formatPhone } from "@skelet/storefront-next/forms/phone";

type OrderEvent = { id: string; message: string; createdAt: string };

export type OrderDetailOrder = OrderSummaryFieldsFragment & {
  events?: OrderEvent[];
};

type Address = OrderSummaryFieldsFragment["shippingAddress"];

export function OrderDetailView({
  order,
  headerAction,
  extraSections,
  paymentIncomplete = false,
}: {
  order: OrderDetailOrder;
  headerAction?: ReactNode;
  extraSections?: ReactNode;
  // Checkout placed the order but its payment wanted an action the storefront can't run
  paymentIncomplete?: boolean;
}) {
  const t = useTranslations("order");
  const tc = useTranslations("common");
  const tendered = order.giftCardAmount + order.storeCreditAmount;
  return (
    <>
      <header className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
            {t("label")}
          </p>
          <h1 className="mt-1 font-mono text-3xl font-extrabold tracking-tight">
            {order.number}
          </h1>
          <div className="mt-3 flex flex-wrap items-center gap-2 text-xs">
            <StatusChip>{humanizeStatus(order.status)}</StatusChip>
            <PaymentChip status={order.paymentStatus} />
            <span className="text-muted-foreground">
              {t("placed", { date: new Date(order.createdAt).toLocaleDateString() })}
            </span>
          </div>
        </div>
        {headerAction}
      </header>

      {paymentIncomplete ? (
        <section role="status" className="mt-6 rounded-xl border border-amber-300 bg-amber-50 p-5 text-sm">
          <p className="font-semibold">{t("paymentIncompleteTitle")}</p>
          <p className="mt-1 text-muted-foreground">{t("paymentIncompleteBody")}</p>
        </section>
      ) : null}

      {order.paymentInstructions ? (
        <section className="mt-6 rounded-xl border border-amber-300 bg-amber-50 p-5 text-sm">
          <p className="font-semibold">{t("paymentPending")}</p>
          <p className="mt-1 whitespace-pre-line text-muted-foreground">
            {order.paymentInstructions}
          </p>
        </section>
      ) : null}

      {(order.carrier || order.trackingNumber) && (
        <section className="glass mt-6 flex items-start gap-3 rounded-xl p-5 text-sm">
          <TruckIcon
            className="mt-0.5 size-5 shrink-0 text-signal"
            aria-hidden="true"
          />
          <div>
            <p className="font-semibold">
              {t("shippedVia", { carrier: order.carrier || t("carrierFallback") })}
              {order.trackingNumber ? (
                <>
                  {" · "}
                  {order.trackingUrl ? (
                    <a
                      href={order.trackingUrl}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="font-mono underline underline-offset-2"
                    >
                      {order.trackingNumber}
                    </a>
                  ) : (
                    <code className="font-mono">{order.trackingNumber}</code>
                  )}
                </>
              ) : null}
            </p>
            <p className="mt-1 text-xs text-muted-foreground">
              {t("trackingDelay")}
            </p>
          </div>
        </section>
      )}

      {/* Items + money in one card - the receipt people actually scan */}
      <section className="glass mt-6 rounded-xl p-5">
        <h2 className="flex items-center gap-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">
          <PackageIcon className="size-4" aria-hidden="true" />
          {t("items", { count: order.lines.length })}
        </h2>
        <ul className="mt-3 divide-y divide-border">
          {order.lines.map((line) => {
            const name = displayName(line.name, line.sku);
            // Lines placed against a product with no name fall back to the SKU, so
            // don't print the same string twice
            const showSku = name !== line.sku;
            return (
            <li key={line.id} className="flex items-start gap-3 py-3">
              <div className="min-w-0 flex-1">
                <p className="text-sm font-semibold">{name}</p>
                <p className="mt-0.5 text-xs text-muted-foreground">
                  {showSku ? `${line.sku} · ` : ""}
                  {formatPrice(line.unitPrice, order.currencyCode)} ×{" "}
                  {line.quantity}
                </p>
                {line.options.length > 0 && (
                  <ul className="mt-1.5 space-y-0.5 text-xs text-muted-foreground">
                    {line.options.map((o) => (
                      <li key={o.id}>
                        {o.inputType === "rule" ? (
                          <span className="text-foreground">{o.optionTitle}</span>
                        ) : (
                          <>{o.optionTitle}:{" "}</>
                        )}
                        {o.inputType === "rule" ? null : o.fileUrl ? (
                          <a
                            href={o.fileUrl}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="text-foreground underline"
                          >
                            {o.textValue || tc("download")}
                          </a>
                        ) : (
                          <span className="text-foreground">
                            {o.valueLabel || o.textValue}
                          </span>
                        )}
                        {o.priceDelta !== 0 &&
                          ` (${o.priceDelta > 0 ? "+" : ""}${formatPrice(o.priceDelta, order.currencyCode)})`}
                      </li>
                    ))}
                  </ul>
                )}
              </div>
              <span className="text-sm font-semibold tabular-nums">
                {formatPrice(line.totalPrice, order.currencyCode)}
              </span>
            </li>
            );
          })}
        </ul>

        <dl className="mt-4 space-y-1 border-t border-border pt-4 text-sm">
          <Row label={t("subtotal")}>
            {formatPrice(order.subtotal, order.currencyCode)}
          </Row>
          {order.discountAmount > 0 ? (
            <Row label={t("discount")} className="text-emerald-700">
              −{formatPrice(order.discountAmount, order.currencyCode)}
            </Row>
          ) : null}
          <Row label={t("shipping")}>
            {formatPrice(order.shippingAmount, order.currencyCode)}
          </Row>
          <Row label={t("tax")}>
            {formatPrice(order.taxAmount, order.currencyCode)}
          </Row>
        </dl>
        <p className="mt-3 flex items-baseline justify-between border-t border-border pt-3 text-lg font-bold">
          <span>{t("total")}</span>
          <span className="tabular-nums">
            {formatPrice(order.total, order.currencyCode)}
          </span>
        </p>

        {/* Gift cards and store credit are tenders, not discounts: they leave
            `total` alone and pay it down. Without these rows a customer who
            covered part of an order with credit just sees the gross total. */}
        {tendered > 0 && (
          <dl className="mt-3 space-y-1 border-t border-border pt-3 text-sm">
            {order.giftCardAmount > 0 && (
              <Row label={t("paidGiftCard")} className="text-emerald-700">
                −{formatPrice(order.giftCardAmount, order.currencyCode)}
              </Row>
            )}
            {order.storeCreditAmount > 0 && (
              <Row label={t("paidStoreCredit")} className="text-emerald-700">
                −{formatPrice(order.storeCreditAmount, order.currencyCode)}
              </Row>
            )}
            <div className="flex justify-between gap-4 pt-1 font-semibold">
              <dt>{t("amountDue")}</dt>
              <dd className="tabular-nums">
                {formatPrice(order.amountDue, order.currencyCode)}
              </dd>
            </div>
          </dl>
        )}
      </section>

      <section className="mt-6 grid gap-4 sm:grid-cols-2">
        <AddressCard title={t("shippingTo")} address={order.shippingAddress} />
        <AddressCard title={t("billing")} address={order.billingAddress} />
      </section>

      {order.customerNote ? (
        <section className="glass mt-6 rounded-xl p-5">
          <h2 className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
            {t("yourNote")}
          </h2>
          <p className="mt-2 whitespace-pre-line text-sm">
            {order.customerNote}
          </p>
        </section>
      ) : null}

      {extraSections}

      {order.events && order.events.length > 0 ? (
        <section className="glass mt-6 rounded-xl p-5">
          <h2 className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
            {t("activity")}
          </h2>
          <ol className="mt-3 space-y-3">
            {order.events.map((ev) => (
              <li key={ev.id} className="flex gap-3 text-sm">
                <span
                  aria-hidden="true"
                  className="mt-1.5 size-1.5 shrink-0 rounded-full bg-primary"
                />
                <span className="flex-1">{ev.message}</span>
                <time
                  dateTime={ev.createdAt}
                  className="shrink-0 text-xs text-muted-foreground"
                >
                  {new Date(ev.createdAt).toLocaleDateString()}
                </time>
              </li>
            ))}
          </ol>
        </section>
      ) : null}
    </>
  );
}

function AddressCard({ title, address }: { title: string; address: Address }) {
  return (
    <div className="glass rounded-xl p-5">
      <h2 className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
        {title}
      </h2>
      <address className="mt-2 text-sm not-italic leading-relaxed">
        {address.firstName} {address.lastName}
        {address.company ? (
          <>
            <br />
            {address.company}
          </>
        ) : null}
        <br />
        {address.street1}
        {address.street2 ? (
          <>
            <br />
            {address.street2}
          </>
        ) : null}
        <br />
        {address.city}
        {address.state ? `, ${address.state}` : ""} {address.postalCode}
        <br />
        {address.countryCode}
        {address.phone ? (
          <>
            <br />
            <span className="text-muted-foreground">{formatPhone(address.phone)}</span>
          </>
        ) : null}
      </address>
    </div>
  );
}

function Row({
  label,
  children,
  className = "",
}: {
  label: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <div className={`flex justify-between gap-4 ${className}`}>
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="tabular-nums">{children}</dd>
    </div>
  );
}
