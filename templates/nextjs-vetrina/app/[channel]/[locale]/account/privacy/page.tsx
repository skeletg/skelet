import { Link } from "@/app/components/LocaleLink";

import { withLocalePrefix } from "@skelet/storefront-next/locale/path";
import { redirect } from "next/navigation";
import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";

import { AccountPageHeader } from "@/app/components/account/AccountUI";
import { getCurrentCustomer } from "@skelet/storefront-next/customer/read";
import { getChannel } from "@skelet/storefront-next/channel/read";
import { supportEmailFrom } from "@skelet/storefront-core/channel/support";

export async function generateMetadata({
  params,
}: PageProps<"/[channel]/[locale]/account/privacy">): Promise<Metadata> {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "account.privacy" });
  return { title: t("title") };
}

export default async function PrivacyRequestsPage({ params }: PageProps<"/[channel]/[locale]/account/privacy">) {
  // Locale straight off the segment - a page taking no props has no request scope for
  // getLocale() to read, so it silently answered with the default language
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "account.privacy" });

  const customer = await getCurrentCustomer();
  if (!customer) redirect(await withLocalePrefix("/account/login?returnTo=/account/privacy", locale));

  const channel = await getChannel();
  const supportEmail = supportEmailFrom(
    channel?.publicUrl,
    process.env.STOREFRONT_SUPPORT_EMAIL,
  );

  const deleteSubject = encodeURIComponent(t("deleteSubject"));
  const deleteBody = encodeURIComponent(
    t("deleteMailBody", { email: customer.email }),
  );

  return (
    <>
      <AccountPageHeader
        title={t("title")}
        description={t("description")}
      />

      <div className="grid gap-4 sm:grid-cols-2">
        <article className="glass flex flex-col rounded-xl p-5">
          <h2 className="font-semibold">{t("exportTitle")}</h2>
          <p className="mt-2 flex-1 text-sm text-muted-foreground">
            {t("exportBody")}
          </p>
          {/* Plain <a> - Link prefetch would fire the export for nothing */}
          {/* eslint-disable-next-line @next/next/no-html-link-for-pages */}
          <a
            href="/account/data-export"
            className="mt-4 inline-flex h-10 items-center justify-center rounded-md border border-border bg-background px-5 text-sm font-semibold transition-colors hover:bg-accent"
          >
            {t("exportCta")}
          </a>
        </article>

        <article className="glass flex flex-col rounded-xl p-5">
          <h2 className="font-semibold">{t("deleteTitle")}</h2>
          <p className="mt-2 flex-1 text-sm text-muted-foreground">
            {t("deleteBody")}
          </p>
          {supportEmail ? (
            <a
              href={`mailto:${supportEmail}?subject=${deleteSubject}&body=${deleteBody}`}
              className="mt-4 inline-flex h-10 items-center justify-center rounded-md border border-red-300 px-5 text-sm font-semibold text-red-700 transition-colors hover:bg-red-50"
            >
              {t("deleteCta")}
            </a>
          ) : (
            <p className="mt-4 text-sm text-muted-foreground">
              {t("deleteByReply")}
            </p>
          )}
        </article>
      </div>

      <section className="glass mt-4 rounded-xl p-5 text-sm">
        <h2 className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
          {t("elsewhere")}
        </h2>
        <ul className="mt-3 space-y-1.5 text-muted-foreground">
          <li>
            {t.rich("updateDetails", {
              link: (chunks) => (
                <Link
                  href="/account/profile"
                  className="text-foreground underline underline-offset-2"
                >
                  {chunks}
                </Link>
              ),
            })}
          </li>
          <li>{t("marketingPrefs")}</li>
          <li>
            {t.rich("readPolicy", {
              link: (chunks) => (
                <Link
                  href="/privacy"
                  className="text-foreground underline underline-offset-2"
                >
                  {chunks}
                </Link>
              ),
            })}
          </li>
        </ul>
      </section>
    </>
  );
}
