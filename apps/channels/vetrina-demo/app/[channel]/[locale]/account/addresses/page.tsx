import { Link } from "@/app/components/LocaleLink";
import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";

import { withLocalePrefix } from "@/app/lib/locale/path";
import { redirect } from "next/navigation";
import { MapPinIcon, PlusIcon } from "lucide-react";

import {
  AccountPageHeader,
  EmptyState,
  FormError,
  buttonPrimary,
} from "@/app/components/account/AccountUI";
import { SubmitButton } from "@/app/components/account/SubmitButton";
import {
  deleteAddressAction,
  setDefaultBillingAction,
  setDefaultShippingAction,
} from "@/app/lib/auth/actions";
import { getCurrentCustomer } from "@/app/lib/customer/read";

export async function generateMetadata({
  params,
}: PageProps<"/[channel]/[locale]/account/addresses">): Promise<Metadata> {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "account.addresses" });
  return { title: t("title") };
}

export default async function AddressesPage({
  params,
  searchParams,
}: PageProps<"/[channel]/[locale]/account/addresses">) {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "account.addresses" });
  const tc = await getTranslations({ locale, namespace: "common" });
  const customer = await getCurrentCustomer();
  if (!customer) redirect(await withLocalePrefix("/account/login?returnTo=/account/addresses", locale));

  const sp = await searchParams;
  const actionError = Array.isArray(sp.error) ? sp.error[0] : sp.error;

  return (
    <>
      <AccountPageHeader
        title={t("title")}
        description={t("description")}
        action={
          <Link href="/account/addresses/new" className={buttonPrimary}>
            <PlusIcon className="mr-1.5 size-4" aria-hidden="true" />
            {t("add")}
          </Link>
        }
      />

      {actionError ? (
        <div className="mb-4">
          <FormError>
            {actionError === "delete" ? t("deleteFailed") : t("defaultFailed")}
          </FormError>
        </div>
      ) : null}

      {customer.addresses.length === 0 ? (
        <EmptyState
          icon={<MapPinIcon className="size-5" aria-hidden="true" />}
          title={t("emptyTitle")}
          description={t("emptyBody")}
          actionHref="/account/addresses/new"
          actionLabel={t("add")}
        />
      ) : (
        <ul className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          {customer.addresses.map((a) => {
            const isDefaultBilling =
              a.id === customer.defaultBillingAddress?.id;
            const isDefaultShipping =
              a.id === customer.defaultShippingAddress?.id;
            return (
              <li key={a.id} className="glass flex flex-col rounded-xl p-5">
                {(isDefaultBilling || isDefaultShipping) && (
                  <p className="mb-3 flex flex-wrap gap-1.5">
                    {isDefaultBilling && <DefaultTag>{t("defaultBillingTag")}</DefaultTag>}
                    {isDefaultShipping && <DefaultTag>{t("defaultShippingTag")}</DefaultTag>}
                  </p>
                )}
                <address className="text-sm not-italic leading-relaxed">
                  <span className="font-semibold">
                    {a.firstName} {a.lastName}
                  </span>
                  {a.company ? (
                    <>
                      <br />
                      <span className="text-muted-foreground">{a.company}</span>
                    </>
                  ) : null}
                  <br />
                  {a.street1}
                  {a.street2 ? (
                    <>
                      <br />
                      {a.street2}
                    </>
                  ) : null}
                  <br />
                  {a.city}
                  {a.state ? `, ${a.state}` : ""} {a.postalCode}
                  <br />
                  {a.countryCode}
                  {a.phone ? (
                    <>
                      <br />
                      <span className="text-muted-foreground">{a.phone}</span>
                    </>
                  ) : null}
                </address>

                <div className="mt-auto flex flex-wrap items-center gap-x-4 gap-y-2 border-t border-border pt-3 text-xs">
                  <Link
                    href={`/account/addresses/${a.id}/edit`}
                    className="font-semibold hover:underline"
                  >
                    {tc("edit")}
                  </Link>
                  {!isDefaultBilling && (
                    <form action={setDefaultBillingAction}>
                      <input type="hidden" name="id" value={a.id} />
                      <SubmitButton
                        className="text-muted-foreground hover:text-foreground hover:underline"
                        pendingLabel={tc("saving")}
                      >
                        {t("makeDefaultBilling")}
                      </SubmitButton>
                    </form>
                  )}
                  {!isDefaultShipping && (
                    <form action={setDefaultShippingAction}>
                      <input type="hidden" name="id" value={a.id} />
                      <SubmitButton
                        className="text-muted-foreground hover:text-foreground hover:underline"
                        pendingLabel={tc("saving")}
                      >
                        {t("makeDefaultShipping")}
                      </SubmitButton>
                    </form>
                  )}
                  <form action={deleteAddressAction} className="ml-auto">
                    <input type="hidden" name="id" value={a.id} />
                    <SubmitButton
                      className="text-red-600 hover:underline"
                      confirmLabel={t("confirmDelete")}
                      pendingLabel={tc("deleting")}
                    >
                      {tc("delete")}
                    </SubmitButton>
                  </form>
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </>
  );
}

function DefaultTag({ children }: { children: string }) {
  return (
    <span className="rounded-sm bg-foreground px-2.5 py-0.5 text-[0.65rem] font-semibold uppercase tracking-wider text-background">
      {children}
    </span>
  );
}
