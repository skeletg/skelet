import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";
import { redirect } from "next/navigation";

import { withLocalePrefix } from "@skelet/storefront-next/locale/path";
import { DownloadIcon } from "lucide-react";

import {
  AccountCard,
  AccountPageHeader,
  EmptyState,
} from "@/app/components/account/AccountUI";
import { getCurrentCustomer } from "@skelet/storefront-next/customer/read";
import { gqlFetch, loadErrorMessage } from "@skelet/storefront-next/graphql";
import { MyDownloadsDocument } from "@skelet/storefront-core/customer/operations.gql";

export async function generateMetadata({
  params,
}: PageProps<"/[channel]/[locale]/account/downloads">): Promise<Metadata> {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "account.downloads" });
  return { title: t("title") };
}

type DownloadGrant = {
  id: string;
  title: string;
  productId: string;
  downloadUrl: string;
  downloadsUsed: number;
  maxDownloads: number | null;
  expiresAt: string | null;
  createdAt: string;
};

export default async function DownloadsPage({ params }: PageProps<"/[channel]/[locale]/account/downloads">) {
  // Locale straight off the segment - a page taking no props has no request scope for
  // getLocale() to read, so it silently answered with the default language
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "account.downloads" });
  const tc = await getTranslations({ locale, namespace: "common" });

  const customer = await getCurrentCustomer();
  if (!customer) {
    redirect(await withLocalePrefix("/account/login?returnTo=/account/downloads", locale));
  }

  let downloads: DownloadGrant[] = [];
  let loadError: string | null = null;
  try {
    const res = await gqlFetch(MyDownloadsDocument);
    downloads = res.data.myDownloads;
  } catch (err) {
    loadError = loadErrorMessage(
      "account/downloads load failed",
      err,
      t("loadFailed"),
    );
  }

  return (
    <>
      <AccountPageHeader
        title={t("title")}
        description={t("description")}
      />

      {loadError ? (
        <p
          role="alert"
          className="rounded-xl border border-red-300 bg-red-50 p-5 text-sm text-red-700"
        >
          {loadError}
        </p>
      ) : downloads.length === 0 ? (
        <EmptyState
          icon={<DownloadIcon className="size-5" aria-hidden="true" />}
          title={t("emptyTitle")}
          description={t("emptyBody")}
          actionHref="/products"
          actionLabel={tc("browseProducts")}
        />
      ) : (
        <AccountCard padded={false}>
          <ul className="divide-y divide-border">
            {downloads.map((d) => {
              const exhausted =
                d.maxDownloads != null && d.downloadsUsed >= d.maxDownloads;
              const expired =
                d.expiresAt != null && new Date(d.expiresAt) <= new Date();
              const available = !exhausted && !expired;
              return (
                <li
                  key={d.id}
                  className="flex items-center justify-between gap-4 px-5 py-4"
                >
                  <div className="min-w-0">
                    <p className="text-sm font-semibold">{d.title}</p>
                    <p className="mt-0.5 text-xs text-muted-foreground">
                      {d.maxDownloads == null
                        ? t("usedUnlimited", { used: d.downloadsUsed })
                        : t("usedOf", { used: d.downloadsUsed, max: d.maxDownloads })}
                      {d.expiresAt
                        ? ` · ${t(expired ? "expiredOn" : "expiresOn", {
                            date: new Date(d.expiresAt).toLocaleDateString(),
                          })}`
                        : ""}
                    </p>
                  </div>
                  {available ? (
                    <a
                      href={d.downloadUrl}
                      className="inline-flex h-10 shrink-0 items-center gap-1.5 rounded-md bg-primary px-5 text-sm font-semibold text-primary-foreground transition-opacity hover:opacity-90"
                    >
                      <DownloadIcon className="size-4" aria-hidden="true" />
                      {tc("download")}
                    </a>
                  ) : (
                    <span className="shrink-0 rounded-sm bg-muted px-4 py-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                      {expired ? t("expired") : t("limitReached")}
                    </span>
                  )}
                </li>
              );
            })}
          </ul>
        </AccountCard>
      )}
    </>
  );
}
