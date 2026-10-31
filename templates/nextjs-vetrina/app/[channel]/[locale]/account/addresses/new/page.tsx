import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";
import { redirect } from "next/navigation";

import { withLocalePrefix } from "@skelet/storefront-next/locale/path";

import {
  AccountCard,
  AccountPageHeader,
} from "@/app/components/account/AccountUI";
import { getAddressFormOptions } from "@skelet/storefront-next/countries/read";
import { getCurrentCustomer } from "@skelet/storefront-next/customer/read";
import { AddressForm } from "@/app/[channel]/[locale]/account/addresses/AddressForm";

export async function generateMetadata({
  params,
}: PageProps<"/[channel]/[locale]/account/addresses/new">): Promise<Metadata> {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "account.addresses" });
  return { title: t("newTitle") };
}

export default async function NewAddressPage({ params }: PageProps<"/[channel]/[locale]/account/addresses/new">) {
  // Locale straight off the segment - a page taking no props has no request scope for
  // getLocale() to read, so it silently answered with the default language
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "account.addresses" });

  const [customer, options] = await Promise.all([getCurrentCustomer(), getAddressFormOptions()]);
  if (!customer) redirect(await withLocalePrefix("/account/login?returnTo=/account/addresses/new", locale));

  return (
    <div className="max-w-2xl">
      <AccountPageHeader
        title={t("newTitle")}
        description={t("newDescription")}
      />
      <AccountCard>
        <AddressForm submitLabel={t("save")} options={options} />
      </AccountCard>
    </div>
  );
}
