import { redirect } from "next/navigation";
import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";
import { withLocalePrefix } from "@skelet/storefront-next/locale/path";

import {
  AuthCard,
  FormError,
  FormSuccess,
} from "@/app/components/account/AccountUI";
import { getCustomerOrNull } from "@skelet/storefront-next/customer/read";
import { safeReturnTo } from "@skelet/storefront-core/auth/return-to";
import { LoginForm } from "@/app/components/auth/LoginForm";
import { SsoSignIn } from "@/app/components/auth/SsoSignIn";
import { ssoLabel } from "@skelet/storefront-next/auth/sso";

export async function generateMetadata({
  params,
}: PageProps<"/[channel]/[locale]/account/login">): Promise<Metadata> {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "auth.login" });
  return { title: t("title") };
}

export default async function LoginPage({
  params,
  searchParams,
}: PageProps<"/[channel]/[locale]/account/login">) {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "auth.login" });
  const sp = await searchParams;
  const returnTo = safeReturnTo(sp.returnTo);
  const justReset =
    (Array.isArray(sp.reset) ? sp.reset[0] : sp.reset) === "ok";
  const verify = Array.isArray(sp.verify) ? sp.verify[0] : sp.verify;
  const needsVerify = verify === "required";
  const verifySent = verify === "sent";
  const ssoOutcome = Array.isArray(sp.sso) ? sp.sso[0] : sp.sso;
  const ssoProblem = ssoOutcome === "unverified" ? "ssoUnverified" : ssoOutcome === "cancelled" ? "ssoCancelled" : ssoOutcome ? "ssoFailed" : null;

  const [customer, sso] = await Promise.all([getCustomerOrNull(), ssoLabel()]);
  if (customer) redirect(await withLocalePrefix(returnTo ?? "/account", locale));

  return (
    <AuthCard
      title={t("title")}
      description={t("description")}
      banner={
        ssoProblem ? (
          <FormError>{t(ssoProblem)}</FormError>
        ) : needsVerify ? (
          <FormSuccess>{t("verifyBanner")}</FormSuccess>
        ) : verifySent ? (
          <FormSuccess>{t("verifySentBanner")}</FormSuccess>
        ) : justReset ? (
          <FormSuccess>{t("resetBanner")}</FormSuccess>
        ) : undefined
      }
    >
      <LoginForm returnTo={returnTo} />
      {sso && <SsoSignIn label={sso} returnTo={returnTo} />}
    </AuthCard>
  );
}
