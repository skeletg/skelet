import { Link } from "@/app/components/LocaleLink";
import { getLocale } from "next-intl/server";

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

export const metadata = { title: "Addresses" };

export default async function AddressesPage({
  searchParams,
}: {
  searchParams: Promise<{ error?: string | string[] }>;
}) {
  const customer = await getCurrentCustomer();
  if (!customer) redirect(await withLocalePrefix("/account/login?returnTo=/account/addresses", await getLocale()));

  const sp = await searchParams;
  const actionError = Array.isArray(sp.error) ? sp.error[0] : sp.error;

  return (
    <>
      <AccountPageHeader
        title="Addresses"
        description="Saved addresses show up as one-tap choices at checkout."
        action={
          <Link href="/account/addresses/new" className={buttonPrimary}>
            <PlusIcon className="mr-1.5 size-4" aria-hidden="true" />
            Add address
          </Link>
        }
      />

      {actionError ? (
        <div className="mb-4">
          <FormError>
            {actionError === "delete"
              ? "We couldn't delete that address. Please try again."
              : "We couldn't change your default address. Please try again."}
          </FormError>
        </div>
      ) : null}

      {customer.addresses.length === 0 ? (
        <EmptyState
          icon={<MapPinIcon className="size-5" aria-hidden="true" />}
          title="No saved addresses"
          description="Add one here, or save it while checking out - either way it lands on this page."
          actionHref="/account/addresses/new"
          actionLabel="Add address"
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
                    {isDefaultBilling && <DefaultTag>Billing</DefaultTag>}
                    {isDefaultShipping && <DefaultTag>Shipping</DefaultTag>}
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
                    Edit
                  </Link>
                  {!isDefaultBilling && (
                    <form action={setDefaultBillingAction}>
                      <input type="hidden" name="id" value={a.id} />
                      <SubmitButton
                        className="text-muted-foreground hover:text-foreground hover:underline"
                        pendingLabel="Saving…"
                      >
                        Default billing
                      </SubmitButton>
                    </form>
                  )}
                  {!isDefaultShipping && (
                    <form action={setDefaultShippingAction}>
                      <input type="hidden" name="id" value={a.id} />
                      <SubmitButton
                        className="text-muted-foreground hover:text-foreground hover:underline"
                        pendingLabel="Saving…"
                      >
                        Default shipping
                      </SubmitButton>
                    </form>
                  )}
                  <form action={deleteAddressAction} className="ml-auto">
                    <input type="hidden" name="id" value={a.id} />
                    <SubmitButton
                      className="text-red-600 hover:underline"
                      confirmLabel="Confirm delete"
                      pendingLabel="Deleting…"
                    >
                      Delete
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
      Default {children}
    </span>
  );
}
