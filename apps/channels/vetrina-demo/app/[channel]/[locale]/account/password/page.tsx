import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";
import { redirect } from "next/navigation";

import { withLocalePrefix } from "@/app/lib/locale/path";

import {
  AccountCard,
  AccountPageHeader,
} from "@/app/components/account/AccountUI";
import { getCurrentCustomer } from "@/app/lib/customer/read";
import { PasswordForm } from "./PasswordForm";

export async function generateMetadata({
  params,
}: PageProps<"/[channel]/[locale]/account/password">): Promise<Metadata> {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "account.password" });
  return { title: t("title") };
}

export default async function ChangePasswordPage({ params }: PageProps<"/[channel]/[locale]/account/password">) {
  // Locale straight off the segment - a page taking no props has no request scope for
  // getLocale() to read, so it silently answered with the default language
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "account.password" });

  const customer = await getCurrentCustomer();
  if (!customer) redirect(await withLocalePrefix("/account/login?returnTo=/account/password", locale));
  return (
    <div className="max-w-xl">
      <AccountPageHeader
        title={t("title")}
        description={t("description")}
      />
      <AccountCard>
        <PasswordForm />
      </AccountCard>
    </div>
  );
}
