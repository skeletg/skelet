import { Link } from "@/app/components/LocaleLink";
import { getLocale } from "next-intl/server";

import { withLocalePrefix } from "@/app/lib/locale/path";
import { redirect } from "next/navigation";
import {
  CreditCardIcon,
  DownloadIcon,
  HeartIcon,
  MapPinIcon,
  PackageIcon,
  RefreshCwIcon,
} from "lucide-react";

import { AnalyticsEvent } from "@/app/components/AnalyticsEvent";
import {
  AccountCard,
  AccountPageHeader,
  FormError,
  FormSuccess,
  buttonSecondary,
  inputClass,
} from "@/app/components/account/AccountUI";
import { SubmitButton } from "@/app/components/account/SubmitButton";
import { PaymentChip } from "@/app/components/account/StatusChip";
import { resendVerificationEmailAction } from "@/app/lib/auth/actions";
import { getResolvedConsent } from "@/app/lib/consent/read";
import { getChannel } from "@/app/lib/channel/read";
import { getCurrentCustomer } from "@/app/lib/customer/read";
import {
  getLoyaltyStatus,
  getMyReferral,
  redeemLoyaltyPointsAction,
} from "@/app/lib/loyalty/actions";
import { gqlFetch } from "@/app/lib/graphql";
import { MyOrdersDocument } from "@/app/lib/checkout/operations.gql";
import { formatPrice } from "@/app/lib/format";

export const metadata = { title: "Account" };

const RECENT_ORDER_COUNT = 3;

export default async function AccountPage({
  searchParams,
}: {
  searchParams: Promise<{
    welcome?: string | string[];
    resent?: string | string[];
    error?: string | string[];
    done?: string | string[];
  }>;
}) {
  const customer = await getCurrentCustomer();
  if (!customer) redirect(await withLocalePrefix("/account/login", await getLocale()));
  const channel = await getChannel();
  const currency = channel?.currencyCode ?? "USD";
  const [loyalty, referral, orders] = await Promise.all([
    getLoyaltyStatus(),
    getMyReferral(),
    loadRecentOrders(),
  ]);
  const sp = await searchParams;
  const welcomeRaw = Array.isArray(sp.welcome) ? sp.welcome[0] : sp.welcome;
  const welcome = welcomeRaw === "1";
  const resent = Array.isArray(sp.resent) ? sp.resent[0] : sp.resent;
  const actionError = Array.isArray(sp.error) ? sp.error[0] : sp.error;
  const actionDone = Array.isArray(sp.done) ? sp.done[0] : sp.done;
  const consent = welcome ? await getResolvedConsent() : null;

  const loyaltyWorth =
    loyalty?.enabled && loyalty.balancePoints > 0
      ? formatPrice(loyalty.balancePoints * loyalty.redeemCentsPerPoint, currency)
      : null;

  return (
    <>
      {welcome && consent && (
        <AnalyticsEvent
          enabled={consent.marketing}
          event="sign_up"
          payload={{ method: "password" }}
          stripParam="welcome"
        />
      )}

      <AccountPageHeader
        title={`Hi, ${customer.firstName}`}
        description="Everything you've bought and every detail we hold, in one place."
      />

      {actionError === "redeem" ? (
        <div className="mb-6">
          <FormError>
            We couldn&rsquo;t redeem those points - your balance is unchanged.
            Please try again.
          </FormError>
        </div>
      ) : actionDone === "redeem" ? (
        <div className="mb-6">
          <FormSuccess>
            Points redeemed - the credit is on your account.
          </FormSuccess>
        </div>
      ) : null}

      {!customer.isEmailVerified && (
        <div className="mb-6 rounded-xl border border-amber-300 bg-amber-50 p-5 text-sm text-amber-900">
          <p className="font-semibold">Confirm your email address</p>
          <p className="mt-1">
            Check your inbox for the confirmation link. Until then some
            features stay locked.
          </p>
          {resent === "1" ? (
            <p className="mt-3 font-semibold">
              Sent - give it a minute, then check your inbox.
            </p>
          ) : (
            <form action={resendVerificationEmailAction} className="mt-3">
              <input type="hidden" name="returnTo" value="/account" />
              <SubmitButton
                className="inline-flex h-9 items-center rounded-md border border-amber-400 px-4 text-xs font-semibold hover:bg-amber-100"
                pendingLabel="Sending…"
              >
                {resent === "0"
                  ? "Sending failed - try again"
                  : "Resend verification email"}
              </SubmitButton>
            </form>
          )}
        </div>
      )}

      {/* Money first: the two balances a customer can actually spend. */}
      {(customer.storeCreditBalance > 0 || loyalty?.enabled) && (
        <div className="mb-6 grid gap-4 sm:grid-cols-2">
          {customer.storeCreditBalance > 0 && (
            <AccountCard title="Store credit">
              <p className="mt-2 text-3xl font-semibold tabular-nums">
                {formatPrice(customer.storeCreditBalance, currency)}
              </p>
              <p className="mt-1 text-xs text-muted-foreground">
                Tick &ldquo;use my store credit&rdquo; at checkout to spend it.
              </p>
            </AccountCard>
          )}
          {loyalty?.enabled && (
            <AccountCard title="Loyalty points">
              <p className="mt-2 text-3xl font-semibold tabular-nums">
                {loyalty.balancePoints.toLocaleString()}
              </p>
              <p className="mt-1 text-xs text-muted-foreground">
                {loyaltyWorth
                  ? `Worth ${loyaltyWorth} in store credit. `
                  : ""}
                Earn {loyalty.earnPointsPerUnit} point
                {loyalty.earnPointsPerUnit === 1 ? "" : "s"} per{" "}
                {formatPrice(100, currency)} spent.
              </p>
              {loyalty.balancePoints > 0 && (
                <form
                  action={redeemLoyaltyPointsAction}
                  className="mt-4 flex flex-wrap items-center gap-2"
                >
                  <input
                    type="number"
                    name="points"
                    min={1}
                    max={loyalty.balancePoints}
                    defaultValue={loyalty.balancePoints}
                    aria-label="Points to redeem"
                    className={`${inputClass} w-28`}
                  />
                  <SubmitButton
                    className={buttonSecondary}
                    pendingLabel="Redeeming…"
                  >
                    Redeem for credit
                  </SubmitButton>
                </form>
              )}
            </AccountCard>
          )}
        </div>
      )}

      <AccountCard
        title="Recent orders"
        padded={false}
        action={
          <Link
            href="/account/orders"
            className="text-sm text-muted-foreground hover:text-foreground"
          >
            View all
          </Link>
        }
      >
        {orders.length === 0 ? (
          <p className="px-5 pt-3 pb-5 text-sm text-muted-foreground">
            No orders yet.{" "}
            <Link href="/products" className="underline underline-offset-2">
              Start shopping
            </Link>
            .
          </p>
        ) : (
          <ul className="mt-3 divide-y divide-border border-t border-border">
            {orders.slice(0, RECENT_ORDER_COUNT).map((order) => (
              <li key={order.id}>
                <Link
                  href={`/account/orders/${order.id}`}
                  className="flex items-center justify-between gap-4 px-5 py-3.5 transition-colors hover:bg-accent"
                >
                  <div className="min-w-0">
                    <p className="font-mono text-sm font-semibold">
                      {order.number}
                    </p>
                    <p className="mt-1 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                      {new Date(order.createdAt).toLocaleDateString()}
                      <PaymentChip status={order.paymentStatus} />
                    </p>
                  </div>
                  <span className="shrink-0 text-sm font-semibold tabular-nums">
                    {formatPrice(order.total, order.currencyCode || currency)}
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        )}
      </AccountCard>

      <nav aria-label="Account shortcuts" className="mt-6">
        <ul className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          <Shortcut
            href="/account/orders"
            label="Orders"
            hint="Track, reorder, return"
            Icon={PackageIcon}
          />
          <Shortcut
            href="/account/wishlist"
            label="Wishlist"
            hint="Saved for later"
            Icon={HeartIcon}
          />
          <Shortcut
            href="/account/addresses"
            label="Addresses"
            hint={`${customer.addresses.length} saved`}
            Icon={MapPinIcon}
          />
          <Shortcut
            href="/account/payment-methods"
            label="Payment methods"
            hint="Saved cards"
            Icon={CreditCardIcon}
          />
          <Shortcut
            href="/account/downloads"
            label="Downloads"
            hint="Digital purchases"
            Icon={DownloadIcon}
          />
          <Shortcut
            href="/account/subscriptions"
            label="Subscriptions"
            hint="Recurring deliveries"
            Icon={RefreshCwIcon}
          />
        </ul>
      </nav>

      {referral?.enabled && (
        <AccountCard title="Refer a friend" className="mt-6">
          <p className="mt-2 text-sm">
            Share your link - they get{" "}
            {formatPrice(referral.refereeRewardCents, currency)} in store credit
            on their first order, you get{" "}
            {formatPrice(referral.referrerRewardCents, currency)}.
          </p>
          <p className="mt-3 rounded-lg bg-muted px-3 py-2 font-mono text-xs break-all">
            {(channel?.publicUrl ?? "").replace(/\/$/, "")}
            /account/register?ref={referral.code}
          </p>
          {referral.referredCount > 0 && (
            <p className="mt-2 text-xs text-muted-foreground">
              {referral.referredCount} referred · {referral.rewardedCount}{" "}
              converted · {formatPrice(referral.earnedCents, currency)} earned
            </p>
          )}
        </AccountCard>
      )}
    </>
  );
}

async function loadRecentOrders() {
  try {
    const res = await gqlFetch(MyOrdersDocument);
    return res.data.myOrders;
  } catch {
    // The overview is a summary - a failed order fetch shouldn't take the
    // whole page down. /account/orders reports the error properly
    return [];
  }
}

function Shortcut({
  href,
  label,
  hint,
  Icon,
}: {
  href: string;
  label: string;
  hint: string;
  Icon: typeof PackageIcon;
}) {
  return (
    <li>
      <Link
        href={href}
        className="glass flex items-center gap-3 rounded-xl p-4 transition-colors hover:bg-accent"
      >
        <span className="flex size-10 shrink-0 items-center justify-center rounded-full bg-muted">
          <Icon className="size-4" aria-hidden="true" />
        </span>
        <span className="min-w-0">
          <span className="block text-sm font-semibold">{label}</span>
          <span className="block truncate text-xs text-muted-foreground">
            {hint}
          </span>
        </span>
      </Link>
    </li>
  );
}
