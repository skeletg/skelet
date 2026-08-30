import { Link } from "@/app/components/LocaleLink";

import { withLocalePrefix } from "@/app/lib/locale/path";
import { redirect } from "next/navigation";

import { AccountPageHeader } from "@/app/components/account/AccountUI";
import { getCurrentCustomer } from "@/app/lib/customer/read";
import { getChannel } from "@/app/lib/channel/read";
import { supportEmailFrom } from "@/app/lib/channel/support";

export const metadata = { title: "Data & privacy" };

export default async function PrivacyRequestsPage({ params }: PageProps<"/[locale]/account/privacy">) {
  // Locale straight off the segment - a page taking no props has no request scope for
  // getLocale() to read, so it silently answered with the default language
  const { locale } = await params;

  const customer = await getCurrentCustomer();
  if (!customer) redirect(await withLocalePrefix("/account/login?returnTo=/account/privacy", locale));

  const channel = await getChannel();
  const supportEmail = supportEmailFrom(
    channel?.publicUrl,
    process.env.STOREFRONT_SUPPORT_EMAIL,
  );

  const deleteSubject = encodeURIComponent("Account deletion request");
  const deleteBody = encodeURIComponent(
    `Hi,\n\nPlease delete the personal data associated with my account (${customer.email}), keeping only what you're legally required to retain (e.g. accounting records for past orders).\n\nThanks.`,
  );

  return (
    <>
      <AccountPageHeader
        title="Data & privacy"
        description="You can get a copy of everything we hold on you, correct it, restrict how we use it, or ask us to delete it."
      />

      <div className="grid gap-4 sm:grid-cols-2">
        <article className="glass flex flex-col rounded-xl p-5">
          <h2 className="font-semibold">Export my data</h2>
          <p className="mt-2 flex-1 text-sm text-muted-foreground">
            A JSON copy of your profile, addresses and order history,
            generated on the spot.
          </p>
          {/* Plain <a> - Link prefetch would fire the export for nothing */}
          {/* eslint-disable-next-line @next/next/no-html-link-for-pages */}
          <a
            href="/account/data-export"
            className="mt-4 inline-flex h-10 items-center justify-center rounded-md border border-border bg-background px-5 text-sm font-semibold transition-colors hover:bg-accent"
          >
            Download my data
          </a>
        </article>

        <article className="glass flex flex-col rounded-xl p-5">
          <h2 className="font-semibold">Delete my account</h2>
          <p className="mt-2 flex-1 text-sm text-muted-foreground">
            Removes your account and personal data. Records tied to
            completed orders are kept for accounting reasons, usually
            7&ndash;10 years. Processed within 30 days.
          </p>
          {supportEmail ? (
            <a
              href={`mailto:${supportEmail}?subject=${deleteSubject}&body=${deleteBody}`}
              className="mt-4 inline-flex h-10 items-center justify-center rounded-md border border-red-300 px-5 text-sm font-semibold text-red-700 transition-colors hover:bg-red-50"
            >
              Request deletion
            </a>
          ) : (
            <p className="mt-4 text-sm text-muted-foreground">
              Reply to any email we&rsquo;ve sent you with the subject
              &ldquo;Account deletion request&rdquo; and we&rsquo;ll take it
              from there.
            </p>
          )}
        </article>
      </div>

      <section className="glass mt-4 rounded-xl p-5 text-sm">
        <h2 className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
          Elsewhere
        </h2>
        <ul className="mt-3 space-y-1.5 text-muted-foreground">
          <li>
            Update your name, phone or addresses from{" "}
            <Link
              href="/account/profile"
              className="text-foreground underline underline-offset-2"
            >
              your profile
            </Link>
            .
          </li>
          <li>
            Change marketing preferences via the cookie banner - your choice
            is saved for 180 days.
          </li>
          <li>
            Read the full{" "}
            <Link
              href="/privacy"
              className="text-foreground underline underline-offset-2"
            >
              privacy policy
            </Link>
            .
          </li>
        </ul>
      </section>
    </>
  );
}
