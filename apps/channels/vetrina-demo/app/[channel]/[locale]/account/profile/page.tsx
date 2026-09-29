import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";
import { redirect } from "next/navigation";

import { withLocalePrefix } from "@/app/lib/locale/path";

import {
  AccountCard,
  AccountPageHeader,
} from "@/app/components/account/AccountUI";
import { getCurrentCustomer } from "@/app/lib/customer/read";
import { ProfileForm } from "./ProfileForm";

export async function generateMetadata({
  params,
}: PageProps<"/[channel]/[locale]/account/profile">): Promise<Metadata> {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "account.profile" });
  return { title: t("title") };
}

export default async function EditProfilePage({ params }: PageProps<"/[channel]/[locale]/account/profile">) {
  // Locale straight off the segment - a page taking no props has no request scope for
  // getLocale() to read, so it silently answered with the default language
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "account.profile" });

  const customer = await getCurrentCustomer();
  if (!customer) redirect(await withLocalePrefix("/account/login?returnTo=/account/profile", locale));
  return (
    <div className="max-w-2xl">
      <AccountPageHeader
        title={t("title")}
        description={t("description")}
      />
      <AccountCard>
        <ProfileForm
          firstName={customer.firstName}
          lastName={customer.lastName}
          phone={customer.phone ?? ""}
          email={customer.email}
        />
      </AccountCard>
    </div>
  );
}
