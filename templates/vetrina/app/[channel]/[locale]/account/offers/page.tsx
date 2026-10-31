import type { Metadata } from "next";
import { getFormatter, getTranslations } from "next-intl/server";
import { redirect } from "next/navigation";
import { TagIcon } from "lucide-react";

import { AccountCard, AccountPageHeader, EmptyState } from "@/app/components/account/AccountUI";
import { openAccountOffer } from "@skelet/storefront-next/cart/offer-actions";
import { MyCartOffersDocument, type MyCartOffersQuery } from "@skelet/storefront-next/cart/offer.gql";
import { getChannel } from "@skelet/storefront-next/channel/read";
import { getCurrentCustomer } from "@skelet/storefront-next/customer/read";
import { formatPrice } from "@skelet/storefront-next/format";
import { gqlFetch, loadErrorMessage } from "@skelet/storefront-next/graphql";
import { withLocalePrefix } from "@skelet/storefront-next/locale/path";

export async function generateMetadata({ params }: PageProps<"/[channel]/[locale]/account/offers">): Promise<Metadata> {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "account.offers" });
  return { title: t("title") };
}

// The offers the store chose to list in the customer's account - each opens like its emailed link would
export default async function OffersPage({ params }: PageProps<"/[channel]/[locale]/account/offers">) {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "account.offers" });
  const to = await getTranslations({ locale, namespace: "offer" });
  const format = await getFormatter({ locale });

  const customer = await getCurrentCustomer();
  if (!customer) {
    redirect(await withLocalePrefix("/account/login?returnTo=/account/offers", locale));
  }
  const currency = (await getChannel())?.currency ?? "USD";

  let list: MyCartOffersQuery["myCartOffers"] = [];
  let loadError: string | null = null;
  try {
    list = (await gqlFetch(MyCartOffersDocument)).data.myCartOffers;
  } catch (err) {
    loadError = loadErrorMessage("account/offers load failed", err, to("states.invalid.body"));
  }

  return (
    <>
      <AccountPageHeader title={t("title")} description={t("intro")} />
      {loadError ? (
        <p role="alert" className="rounded-xl border border-border p-5 text-sm">
          {loadError}
        </p>
      ) : list.length === 0 ? (
        <EmptyState icon={<TagIcon className="size-5" aria-hidden="true" />} title={t("empty")} description="" />
      ) : (
        <AccountCard padded={false}>
          <ul className="divide-y divide-border">
            {list.map((o) => (
              <li key={o.id} className="flex items-center justify-between gap-4 px-5 py-4">
                <div className="min-w-0">
                  <p className="text-sm font-semibold tabular-nums">
                    {o.offer?.state === "DRAFT" || o.offer?.state === "SENT" ? (
                      <span className="mr-2 inline-flex rounded-full bg-foreground px-2 py-0.5 text-[10px] font-semibold text-background">{t("new")}</span>
                    ) : null}
                    {formatPrice(Math.max(0, o.subtotal - o.totalDiscount), currency)} · {t("items", { count: o.lines.length })}
                  </p>
                  {o.offer ? (
                    <p className="mt-0.5 text-xs text-muted-foreground">
                      {to("expires", { date: format.dateTime(new Date(o.offer.expiresAt), { dateStyle: "medium", timeStyle: "short" }) })}
                    </p>
                  ) : null}
                  {o.offer?.message ? <p className="mt-1 line-clamp-2 text-xs">{o.offer.message}</p> : null}
                </div>
                <form action={openAccountOffer}>
                  <input type="hidden" name="cartId" value={o.id} />
                  <button
                    type="submit"
                    className="inline-flex h-10 shrink-0 items-center rounded-md bg-primary px-5 text-sm font-semibold text-primary-foreground transition-opacity hover:opacity-90"
                  >
                    {t("open")}
                  </button>
                </form>
              </li>
            ))}
          </ul>
        </AccountCard>
      )}
    </>
  );
}
