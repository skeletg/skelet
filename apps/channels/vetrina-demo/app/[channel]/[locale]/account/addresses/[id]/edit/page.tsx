import { notFound, redirect } from "next/navigation";
import { getLocale } from "next-intl/server";
import { withLocalePrefix } from "@/app/lib/locale/path";
import {
  AccountCard,
  AccountPageHeader,
} from "@/app/components/account/AccountUI";
import { getCurrentCustomer } from "@/app/lib/customer/read";
import { AddressForm } from "@/app/[channel]/[locale]/account/addresses/AddressForm";

export const metadata = { title: "Edit address" };

export default async function EditAddressPage(
  props: PageProps<"/[channel]/[locale]/account/addresses/[id]/edit">,
) {
  const customer = await getCurrentCustomer();
  if (!customer) {
    const { id } = await props.params;
    redirect(
      await withLocalePrefix(
        `/account/login?returnTo=${encodeURIComponent(`/account/addresses/${id}/edit`)}`,
        await getLocale(),
      ),
    );
  }
  const { id } = await props.params;
  const address = customer.addresses.find((a) => a.id === id);
  if (!address) notFound();

  return (
    <div className="max-w-2xl">
      <AccountPageHeader title="Edit address" />
      <AccountCard>
        <AddressForm
          id={address.id}
          submitLabel="Save changes"
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
