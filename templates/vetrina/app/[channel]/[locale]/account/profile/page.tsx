import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";
import { redirect } from "next/navigation";

import { withLocalePrefix } from "@skelet/storefront-next/locale/path";

import {
  AccountCard,
  AccountPageHeader,
} from "@/app/components/account/AccountUI";
import { getPhoneSettings } from "@skelet/storefront-next/countries/read";
import { getCurrentCustomer } from "@skelet/storefront-next/customer/read";
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

  const [customer, phoneSettings] = await Promise.all([getCurrentCustomer(), getPhoneSettings()]);
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
          phoneSettings={phoneSettings}
          email={customer.email}
        />
      </AccountCard>
    </div>
  );
}
