import { Link } from "@/app/components/LocaleLink";
import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";

import { withLocalePrefix } from "@skelet/storefront-next/locale/path";
import { redirect } from "next/navigation";
import {
  CreditCardIcon,
  DownloadIcon,
  HeartIcon,
  MapPinIcon,
  PackageIcon,
  RefreshCwIcon,
  TagIcon,
} from "lucide-react";

import { AccountBadge } from "@/app/components/account/AccountBadge";
import { getAccountCounts } from "@skelet/storefront-next/account/read";
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
import { resendVerificationEmailAction } from "@skelet/storefront-next/auth/actions";
import { getResolvedConsent } from "@skelet/storefront-next/consent/read";
import { getChannel } from "@skelet/storefront-next/channel/read";
import { getCurrentCustomer } from "@skelet/storefront-next/customer/read";
import {
  getLoyaltyStatus,
  getMyReferral,
  redeemLoyaltyPointsAction,
} from "@skelet/storefront-next/loyalty/actions";
import { gqlFetch } from "@skelet/storefront-next/graphql";
import { MyOrdersDocument } from "@skelet/storefront-core/checkout/operations.gql";
import { formatPrice } from "@skelet/storefront-core/format";

export async function generateMetadata({
  params,
}: PageProps<"/[channel]/[locale]/account">): Promise<Metadata> {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "account.overview" });
  return { title: t("title") };
}

const RECENT_ORDER_COUNT = 3;

export default async function AccountPage({
  params,
  searchParams,
}: PageProps<"/[channel]/[locale]/account">) {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "account.overview" });
  const tn = await getTranslations({ locale, namespace: "account.nav" });
  const tc = await getTranslations({ locale, namespace: "common" });
  const customer = await getCurrentCustomer();
  if (!customer) redirect(await withLocalePrefix("/account/login", locale));
  const channel = await getChannel();
  const currency = channel?.currencyCode ?? "USD";
  const [loyalty, referral, orders, counts] = await Promise.all([
    getLoyaltyStatus(),
    getMyReferral(),
    loadRecentOrders(),
    getAccountCounts(),
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
        title={t("greeting", { name: customer.firstName })}
        description={t("description")}
      />

      {actionError === "redeem" ? (
        <div className="mb-6">
          <FormError>{t("redeemFailed")}</FormError>
        </div>
      ) : actionDone === "redeem" ? (
        <div className="mb-6">
          <FormSuccess>{t("redeemDone")}</FormSuccess>
        </div>
      ) : null}

      {!customer.isEmailVerified && (
        <div className="mb-6 rounded-xl border border-amber-300 bg-amber-50 p-5 text-sm text-amber-900">
          <p className="font-semibold">{t("verifyTitle")}</p>
          <p className="mt-1">{t("verifyBody")}</p>
          {resent === "1" ? (
            <p className="mt-3 font-semibold">{t("verifySent")}</p>
          ) : (
            <form action={resendVerificationEmailAction} className="mt-3">
              <input type="hidden" name="returnTo" value="/account" />
              <SubmitButton
                className="inline-flex h-9 items-center rounded-md border border-amber-400 px-4 text-xs font-semibold hover:bg-amber-100"
                pendingLabel={tc("sending")}
              >
                {resent === "0" ? t("verifyResendFailed") : t("verifyResend")}
              </SubmitButton>
            </form>
          )}
        </div>
      )}

      {/* Money first: the two balances a customer can actually spend. */}
      {(customer.storeCreditBalance > 0 || loyalty?.enabled) && (
        <div className="mb-6 grid gap-4 sm:grid-cols-2">
          {customer.storeCreditBalance > 0 && (
            <AccountCard title={t("storeCredit")}>
              <p className="mt-2 text-3xl font-semibold tabular-nums">
                {formatPrice(customer.storeCreditBalance, currency)}
              </p>
              <p className="mt-1 text-xs text-muted-foreground">
                {t("storeCreditHint")}
              </p>
            </AccountCard>
          )}
          {loyalty?.enabled && (
            <AccountCard title={t("loyalty")}>
              <p className="mt-2 text-3xl font-semibold tabular-nums">
                {loyalty.balancePoints.toLocaleString()}
              </p>
              <p className="mt-1 text-xs text-muted-foreground">
                {loyaltyWorth ? `${t("loyaltyWorth", { amount: loyaltyWorth })} ` : ""}
                {t("loyaltyEarn", {
                  points: loyalty.earnPointsPerUnit,
                  amount: formatPrice(100, currency),
                })}
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
                    aria-label={t("pointsToRedeem")}
                    className={`${inputClass} w-28`}
                  />
                  <SubmitButton
                    className={buttonSecondary}
                    pendingLabel={t("redeeming")}
                  >
                    {t("redeem")}
                  </SubmitButton>
                </form>
              )}
            </AccountCard>
          )}
        </div>
      )}

      <AccountCard
        title={t("recentOrders")}
        padded={false}
        action={
          <Link
            href="/account/orders"
            className="text-sm text-muted-foreground hover:text-foreground"
          >
            {tc("viewAll")}
          </Link>
        }
      >
        {orders.length === 0 ? (
          <p className="px-5 pt-3 pb-5 text-sm text-muted-foreground">
            {t("noOrders")}{" "}
            <Link href="/products" className="underline underline-offset-2">
              {t("startShopping")}
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

      <nav aria-label={t("shortcuts")} className="mt-6">
        <ul className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          <Shortcut
            href="/account/orders"
            label={tn("orders")}
            hint={t("ordersHint")}
            Icon={PackageIcon}
          />
          <Shortcut
            href="/account/wishlist"
            label={tn("wishlist")}
            hint={t("wishlistHint")}
            count={counts.wishlist}
            countLabel={tn("wishlistCount", { count: counts.wishlist })}
            Icon={HeartIcon}
          />
          <Shortcut
            href="/account/offers"
            label={tn("offers")}
            hint={t("offersHint")}
            Icon={TagIcon}
            count={counts.offers}
            countLabel={tn("offerCount", { count: counts.offers })}
            newLabel={counts.newOffers > 0 ? tn("newOffers", { count: counts.newOffers }) : undefined}
          />
          <Shortcut
            href="/account/addresses"
            label={tn("addresses")}
            hint={t("addressesHint", { count: customer.addresses.length })}
            Icon={MapPinIcon}
          />
          <Shortcut
            href="/account/payment-methods"
            label={tn("paymentMethods")}
            hint={t("paymentMethodsHint")}
            Icon={CreditCardIcon}
          />
          <Shortcut
            href="/account/downloads"
            label={tn("downloads")}
            hint={t("downloadsHint")}
            Icon={DownloadIcon}
          />
          <Shortcut
            href="/account/subscriptions"
            label={tn("subscriptions")}
            hint={t("subscriptionsHint")}
            Icon={RefreshCwIcon}
          />
        </ul>
      </nav>

      {referral?.enabled && (
        <AccountCard title={t("referTitle")} className="mt-6">
          <p className="mt-2 text-sm">
            {t("referBody", {
              referee: formatPrice(referral.refereeRewardCents, currency),
              referrer: formatPrice(referral.referrerRewardCents, currency),
            })}
          </p>
          <p className="mt-3 rounded-lg bg-muted px-3 py-2 font-mono text-xs break-all">
            {(channel?.publicUrl ?? "").replace(/\/$/, "")}
            /account/register?ref={referral.code}
          </p>
          {referral.referredCount > 0 && (
            <p className="mt-2 text-xs text-muted-foreground">
              {t("referStats", {
                referred: referral.referredCount,
                converted: referral.rewardedCount,
                earned: formatPrice(referral.earnedCents, currency),
              })}
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
  count = 0,
  countLabel = "",
  newLabel,
}: {
  href: string;
  label: string;
  hint: string;
  Icon: typeof PackageIcon;
  count?: number;
  countLabel?: string;
  newLabel?: string;
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
        <AccountBadge count={count} label={countLabel} newLabel={newLabel} />
      </Link>
    </li>
  );
}
