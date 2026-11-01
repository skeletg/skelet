// Landing page for the emailed verification link (<storefront>/verify-email?token=...)

import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";

import { Link } from "@/app/components/LocaleLink";

import { resendVerificationEmailAction } from "@skelet/storefront-next/auth/actions";
import { VerifyEmailDocument } from "@skelet/storefront-core/auth/operations.gql";
import { getCustomerOrNull } from "@skelet/storefront-next/customer/read";
import { gqlFetch } from "@skelet/storefront-next/graphql";

export async function generateMetadata({
  params,
}: PageProps<"/[channel]/[locale]/verify-email">): Promise<Metadata> {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "verifyEmail" });
  return { title: t("title"), robots: { index: false, follow: false } };
}

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
  params,
  searchParams,
}: PageProps<"/[channel]/[locale]/verify-email">) {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "verifyEmail" });
  const sp: RawSearchParams = await searchParams;
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
              {t("inboxTitle")}
            </h1>
            <p className="mt-3 text-sm text-muted-foreground">{t("inboxBody")}</p>
          </>
        ) : (
          <>
            <h1 className="text-2xl font-extrabold tracking-tight">
              {t("sendFailedTitle")}
            </h1>
            <p className="mt-3 text-sm text-muted-foreground">{t("sendFailedBody")}</p>
            <Link href="/account" className={`${pillCta} mt-6`}>
              {t("goToAccount")}
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
          {t("title")}
        </h1>
        <p className="mt-3 text-sm text-muted-foreground">{t("noTokenBody")}</p>
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
          {t("verifiedTitle")}
        </h1>
        <p className="mt-3 text-sm text-muted-foreground">{t("verifiedBody")}</p>
        <Link href="/account" className={`${pillCta} mt-6`}>
          {t("goToAccount")}
        </Link>
      </Card>
    );
  }

  return (
    <Card>
      <h1 className="text-2xl font-extrabold tracking-tight">
        {t("failedTitle")}
      </h1>
      <p className="mt-3 text-sm text-muted-foreground">{t("failedBody")}</p>
      <ResendBlock />
    </Card>
  );
}

// Resend affordance: a one-click server action when signed in, a pointer to login when
// not (the mutation needs a bearer token)
async function ResendBlock() {
  const [customer, t] = await Promise.all([
    getCustomerOrNull(),
    getTranslations("verifyEmail"),
  ]);
  if (!customer) {
    return (
      <p className="mt-6 text-sm text-muted-foreground">
        {t.rich("signInToResend", {
          link: (chunks) => (
            <Link
              href={`/account/login?returnTo=${encodeURIComponent("/verify-email")}`}
              className="text-foreground underline underline-offset-2"
            >
              {chunks}
            </Link>
          ),
        })}
      </p>
    );
  }
  if (customer.isEmailVerified) {
    return (
      <div className="mt-6">
        <p className="text-sm text-muted-foreground">
          {t("alreadyVerified", { email: customer.email })}
        </p>
        <Link href="/account" className={`${pillCta} mt-4`}>
          {t("goToAccount")}
        </Link>
      </div>
    );
  }
  return (
    <form action={resendVerificationEmailAction} className="mt-6">
      <input type="hidden" name="returnTo" value="/verify-email" />
      <button type="submit" className={pillCta}>
        {t("sendNew")}
      </button>
      <p className="mt-3 text-xs text-muted-foreground">
        {t("goesTo", { email: customer.email })}
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
