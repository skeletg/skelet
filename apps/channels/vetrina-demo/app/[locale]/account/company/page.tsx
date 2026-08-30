import { redirect } from "next/navigation";
import { getLocale } from "next-intl/server";

import { withLocalePrefix } from "@/app/lib/locale/path";
import { Building2Icon } from "lucide-react";

import {
  AccountCard,
  AccountPageHeader,
  EmptyState,
  FormError,
  buttonPrimary,
  buttonSecondary,
} from "@/app/components/account/AccountUI";
import { StatusChip } from "@/app/components/account/StatusChip";
import { SubmitButton } from "@/app/components/account/SubmitButton";
import { getCachedCompany } from "@/app/components/account/company";
import {
  getMyCompanyInvoices,
  getMyCompanyOrders,
  getMyQuotes,
  respondToQuoteAction,
} from "@/app/lib/b2b/actions";
import { getChannel } from "@/app/lib/channel/read";
import { getCurrentCustomer } from "@/app/lib/customer/read";
import { formatPrice } from "@/app/lib/format";
import { QuickOrderPad } from "./QuickOrderPad";

export const metadata = { title: "Company account" };

const QUOTE_STATUS_LABEL: Record<string, string> = {
  requested: "Awaiting pricing",
  quoted: "Quoted - respond below",
  accepted: "Accepted",
  declined: "Declined",
  expired: "Expired",
  converted: "Ordered",
};

export default async function CompanyPage({
  searchParams,
}: {
  searchParams: Promise<{ error?: string | string[] }>;
}) {
  const customer = await getCurrentCustomer();
  if (!customer) redirect(await withLocalePrefix("/account/login?returnTo=/account/company", await getLocale()));

  const sp = await searchParams;
  const actionError = Array.isArray(sp.error) ? sp.error[0] : sp.error;

  const [company, quotes, channel] = await Promise.all([
    getCachedCompany(),
    getMyQuotes(),
    getChannel(),
  ]);
  const currency = channel?.currencyCode ?? "USD";

  if (!company && quotes.length === 0) {
    return (
      <>
        <AccountPageHeader title="Company" />
        <EmptyState
          icon={<Building2Icon className="size-5" aria-hidden="true" />}
          title="No company linked"
          description="Contact the store to set up wholesale purchasing, or request a quote straight from your cart any time."
          actionHref="/cart"
          actionLabel="Go to cart"
        />
      </>
    );
  }

  const isCompanyAdmin = company?.role === "admin";
  const [orders, invoices] = await Promise.all([
    isCompanyAdmin ? getMyCompanyOrders() : Promise.resolve([]),
    isCompanyAdmin ? getMyCompanyInvoices() : Promise.resolve([]),
  ]);
  const available = company
    ? company.creditLimitCents - company.creditUsedCents
    : 0;
  const usedPct =
    company && company.creditLimitCents > 0
      ? Math.min(
          100,
          Math.round((company.creditUsedCents / company.creditLimitCents) * 100),
        )
      : 0;

  return (
    <>
      <AccountPageHeader
        title={company?.name ?? "Company"}
        description={
          company
            ? `Net ${company.paymentTermsDays} terms. You are a ${
                company.role === "admin" ? "company admin" : "buyer"
              }.`
            : undefined
        }
      />

      {actionError === "quote" ? (
        <div className="mb-4">
          <FormError>
            We couldn&rsquo;t record your answer - the quote may have expired.
            Please try again.
          </FormError>
        </div>
      ) : null}

      {company && (
        <AccountCard title="Credit line">
          {company.status !== "active" && (
            <p className="mt-2">
              <span className="rounded-sm bg-red-100 px-2.5 py-0.5 text-[0.65rem] font-semibold uppercase tracking-wider text-red-700">
                Account suspended
              </span>
            </p>
          )}
          <p className="mt-3 text-3xl font-semibold tabular-nums">
            {formatPrice(available, currency)}
            <span className="ml-2 text-sm font-normal text-muted-foreground">
              available
            </span>
          </p>
          <div
            className="mt-3 h-2 overflow-hidden rounded-full bg-muted"
            role="img"
            aria-label={`${usedPct}% of credit line used`}
          >
            <div
              className="h-full rounded-full bg-foreground"
              style={{ width: `${usedPct}%` }}
            />
          </div>
          <p className="mt-2 text-sm text-muted-foreground">
            {formatPrice(company.creditUsedCents, currency)} used of{" "}
            {formatPrice(company.creditLimitCents, currency)}. Pay by purchase
            order at checkout.
          </p>
        </AccountCard>
      )}

      <QuickOrderPad />

      <section className="mt-6">
        <h2 className="mb-3 text-xs font-medium uppercase tracking-wider text-muted-foreground">
          Quotes
        </h2>
        {quotes.length === 0 ? (
          <p className="glass rounded-xl p-5 text-sm text-muted-foreground">
            No quotes yet. Fill a cart and use &ldquo;Request a quote&rdquo; to
            start a negotiation.
          </p>
        ) : (
          <ul className="space-y-4">
            {quotes.map((q) => (
              <li key={q.id} className="glass rounded-xl p-5 text-sm">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <StatusChip
                    tone={q.status === "accepted" ? "positive" : "muted"}
                  >
                    {QUOTE_STATUS_LABEL[q.status] ?? q.status}
                  </StatusChip>
                  <p className="text-xs text-muted-foreground">
                    {new Date(q.createdAt).toLocaleDateString()}
                  </p>
                </div>
                <ul className="mt-3 space-y-1 text-muted-foreground">
                  {q.items.map((it) => (
                    <li key={it.sku}>
                      <span className="text-foreground">
                        {it.quantity}× {it.name}
                      </span>
                      {q.status !== "requested" && (
                        <> - {formatPrice(it.unitPriceCents, currency)} each</>
                      )}
                      {it.optionsSummary && (
                        <span className="block pl-4 text-xs">
                          {it.optionsSummary}
                        </span>
                      )}
                    </li>
                  ))}
                </ul>
                {q.totalCents > 0 && (
                  <p className="mt-3 border-t border-border pt-3 font-semibold">
                    Total {formatPrice(q.totalCents, currency)}
                  </p>
                )}
                {q.sellerNote && (
                  <p className="mt-2 text-xs text-muted-foreground">
                    Store: &ldquo;{q.sellerNote}&rdquo;
                  </p>
                )}
                {q.status === "quoted" && (
                  <div className="mt-4 flex flex-wrap gap-2">
                    <form action={respondToQuoteAction}>
                      <input type="hidden" name="id" value={q.id} />
                      <input type="hidden" name="accept" value="1" />
                      <SubmitButton
                        className={buttonPrimary}
                        pendingLabel="Accepting…"
                      >
                        Accept quote
                      </SubmitButton>
                    </form>
                    <form action={respondToQuoteAction}>
                      <input type="hidden" name="id" value={q.id} />
                      <input type="hidden" name="accept" value="0" />
                      <SubmitButton
                        className={buttonSecondary}
                        confirmLabel="Confirm decline"
                        pendingLabel="Declining…"
                      >
                        Decline
                      </SubmitButton>
                    </form>
                  </div>
                )}
                {q.status === "accepted" && (
                  <p className="mt-2 text-xs text-muted-foreground">
                    The store team will turn this into an order shortly.
                  </p>
                )}
              </li>
            ))}
          </ul>
        )}
      </section>

      {invoices.length > 0 && (
        <AccountCard
          title="Invoices"
          padded={false}
          className="mt-6 overflow-hidden"
        >
          <div className="mt-3 overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-y border-border bg-muted/40 text-left text-xs uppercase tracking-wider text-muted-foreground">
                  <th className="px-5 py-2.5 font-medium">Invoice</th>
                  <th className="px-3 py-2.5 font-medium">Order</th>
                  <th className="px-3 py-2.5 font-medium">Status</th>
                  <th className="px-3 py-2.5 font-medium">Due</th>
                  <th className="px-5 py-2.5 text-right font-medium">Amount</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {invoices.map((inv) => (
                  <tr key={inv.number}>
                    <td className="px-5 py-3 font-mono text-xs">
                      {inv.number}
                    </td>
                    <td className="px-3 py-3 font-mono text-xs">
                      {inv.orderNumber}
                    </td>
                    <td className="px-3 py-3">
                      {inv.overdue ? (
                        <span className="font-semibold text-red-700">
                          overdue
                        </span>
                      ) : (
                        inv.status
                      )}
                      {inv.status === "paid" && inv.paidAt && (
                        <span className="text-muted-foreground">
                          {" · "}
                          {new Date(inv.paidAt).toLocaleDateString()}
                        </span>
                      )}
                    </td>
                    <td className="px-3 py-3 text-muted-foreground">
                      {inv.dueAt
                        ? new Date(inv.dueAt).toLocaleDateString()
                        : "-"}
                    </td>
                    <td className="px-5 py-3 text-right tabular-nums">
                      {formatPrice(inv.amountCents, currency)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </AccountCard>
      )}

      {orders.length > 0 && (
        <AccountCard
          title="Company orders"
          padded={false}
          className="mt-6 overflow-hidden"
        >
          <div className="mt-3 overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-y border-border bg-muted/40 text-left text-xs uppercase tracking-wider text-muted-foreground">
                  <th className="px-5 py-2.5 font-medium">Order</th>
                  <th className="px-3 py-2.5 font-medium">Placed by</th>
                  <th className="px-3 py-2.5 font-medium">Status</th>
                  <th className="px-5 py-2.5 text-right font-medium">Total</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {orders.map((o) => (
                  <tr key={o.id}>
                    <td className="px-5 py-3 font-mono text-xs">{o.number}</td>
                    <td className="px-3 py-3">{o.memberEmail}</td>
                    <td className="px-3 py-3">
                      {o.status}
                      <span className="text-muted-foreground">
                        {" · "}
                        {o.paymentStatus}
                      </span>
                    </td>
                    <td className="px-5 py-3 text-right tabular-nums">
                      {formatPrice(o.totalCents, o.currencyCode)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </AccountCard>
      )}
    </>
  );
}
