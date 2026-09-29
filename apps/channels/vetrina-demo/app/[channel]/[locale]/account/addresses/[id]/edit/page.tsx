import { notFound, redirect } from "next/navigation";
import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";
import { withLocalePrefix } from "@/app/lib/locale/path";
import {
  AccountCard,
  AccountPageHeader,
} from "@/app/components/account/AccountUI";
import { getCurrentCustomer } from "@/app/lib/customer/read";
import { AddressForm } from "@/app/[channel]/[locale]/account/addresses/AddressForm";

export async function generateMetadata({
  params,
}: PageProps<"/[channel]/[locale]/account/addresses/[id]/edit">): Promise<Metadata> {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "account.addresses" });
  return { title: t("editTitle") };
}

export default async function EditAddressPage(
  props: PageProps<"/[channel]/[locale]/account/addresses/[id]/edit">,
) {
  const { id, locale } = await props.params;
  const t = await getTranslations({ locale, namespace: "account.addresses" });
  const tc = await getTranslations({ locale, namespace: "common" });
  const customer = await getCurrentCustomer();
  if (!customer) {
    redirect(
      await withLocalePrefix(
        `/account/login?returnTo=${encodeURIComponent(`/account/addresses/${id}/edit`)}`,
        locale,
      ),
    );
  }
  const address = customer.addresses.find((a) => a.id === id);
  if (!address) notFound();

  return (
    <div className="max-w-2xl">
      <AccountPageHeader title={t("editTitle")} />
      <AccountCard>
        <AddressForm
          id={address.id}
          submitLabel={tc("saveChanges")}
          initial={{
            firstName: address.firstName,
            lastName: address.lastName,
            company: address.company,
            street1: address.street1,
            street2: address.street2,
            city: address.city,
            state: address.state,
            postalCode: address.postalCode,
            countryCode: address.countryCode,
            phone: address.phone,
          }}
        />
      </AccountCard>
    </div>
  );
}
