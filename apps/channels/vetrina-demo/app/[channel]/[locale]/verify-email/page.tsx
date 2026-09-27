// Landing page for the emailed verification link (<storefront>/verify-email?token=...)

import { Link } from "@/app/components/LocaleLink";

import { resendVerificationEmailAction } from "@/app/lib/auth/actions";
import { VerifyEmailDocument } from "@/app/lib/auth/operations.gql";
import { getCustomerOrNull } from "@/app/lib/customer/read";
import { gqlFetch } from "@/app/lib/graphql";

export const metadata = {
  title: "Verify your email",
  robots: { index: false, follow: false },
};

export const dynamic = "force-dynamic";

type RawSearchParams = Record<string, string | string[] | undefined>;

function pick(sp: RawSearchParams, key: string): string {
  const raw = sp[key];
  if (typeof raw === "string") return raw.trim();
  if (Array.isArray(raw)) return (raw[0] ?? "").trim();
  return "";
}

const pillCta =
  "inline-flex h-11 items-center justify-center rounded-md bg-primary px-6 " +
  "text-sm font-semibold text-primary-foreground " +
  "hover:bg-primary/90";

export default async function VerifyEmailPage({
  searchParams,
}: {
  searchParams: Promise<RawSearchParams>;
}) {
  const sp = await searchParams;
  const token = pick(sp, "token");
  const resent = pick(sp, "resent");

  // The resend action redirects back here without a token - show the outcome instead of
  // re-attempting a verification
  if (resent) {
    return (
      <Card>
        {resent === "1" ? (
          <>
            <h1 className="text-2xl font-extrabold tracking-tight">
              Check your inbox
            </h1>
            <p className="mt-3 text-sm text-muted-foreground">
              We sent a fresh verification link to your email address.
              Open it on this device to finish up.
            </p>
          </>
        ) : (
          <>
            <h1 className="text-2xl font-extrabold tracking-tight">
              Couldn&rsquo;t send the email
            </h1>
            <p className="mt-3 text-sm text-muted-foreground">
              Something went wrong on our side. Give it a minute, then
              try again from your account page.
            </p>
            <Link href="/account" className={`${pillCta} mt-6`}>
              Go to your account
            </Link>
          </>
        )}
      </Card>
    );
  }

  if (!token) {
    return (
      <Card>
        <h1 className="text-2xl font-extrabold tracking-tight">
          Verify your email
        </h1>
        <p className="mt-3 text-sm text-muted-foreground">
          This page needs the verification link from your email - open
          the most recent message from us and click the button inside.
        </p>
        <ResendBlock />
      </Card>
    );
  }

  let verified = false;
  try {
    const res = await gqlFetch(VerifyEmailDocument, { token });
    verified = res.data.verifyEmail;
  } catch {
    // Invalid / expired / already-used token - fall through to the failure card below
  }

  if (verified) {
    return (
      <Card>
        <h1 className="text-2xl font-extrabold tracking-tight">
          Email verified
        </h1>
        <p className="mt-3 text-sm text-muted-foreground">
          Thanks - your email address is confirmed and your account is
          all set.
        </p>
        <Link href="/account" className={`${pillCta} mt-6`}>
          Go to your account
        </Link>
      </Card>
    );
  }

  return (
    <Card>
      <h1 className="text-2xl font-extrabold tracking-tight">
        That link didn&rsquo;t work
      </h1>
      <p className="mt-3 text-sm text-muted-foreground">
        The verification link is invalid or has expired - they&rsquo;re
        single-use, so a link you already clicked won&rsquo;t work
        twice.
      </p>
      <ResendBlock />
    </Card>
  );
}

// Resend affordance: a one-click server action when signed in, a pointer to login when
// not (the mutation needs a bearer token)
async function ResendBlock() {
  const customer = await getCustomerOrNull();
  if (!customer) {
    return (
      <p className="mt-6 text-sm text-muted-foreground">
        <Link
          href={`/account/login?returnTo=${encodeURIComponent("/verify-email")}`}
          className="text-foreground underline underline-offset-2"
        >
          Sign in
        </Link>{" "}
        to request a new verification link.
      </p>
    );
  }
  if (customer.isEmailVerified) {
    return (
      <div className="mt-6">
        <p className="text-sm text-muted-foreground">
          Good news though - {customer.email} is already verified.
        </p>
        <Link href="/account" className={`${pillCta} mt-4`}>
          Go to your account
        </Link>
      </div>
    );
  }
  return (
    <form action={resendVerificationEmailAction} className="mt-6">
      <input type="hidden" name="returnTo" value="/verify-email" />
      <button type="submit" className={pillCta}>
        Send a new link
      </button>
      <p className="mt-3 text-xs text-muted-foreground">
        Goes to {customer.email}.
      </p>
    </form>
  );
}

function Card({ children }: { children: React.ReactNode }) {
  return (
    <main className="mx-auto w-full max-w-8xl px-6 py-16 md:px-10">
      <div className="glass mx-auto w-full max-w-md rounded-xl p-8">
        {children}
      </div>
    </main>
  );
}
