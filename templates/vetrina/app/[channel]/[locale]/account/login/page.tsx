import { redirect } from "next/navigation";
import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";
import { withLocalePrefix } from "@skelet/storefront-next/locale/path";

import {
  AuthCard,
  FormSuccess,
} from "@/app/components/account/AccountUI";
import { getCustomerOrNull } from "@skelet/storefront-next/customer/read";
import { safeReturnTo } from "@skelet/storefront-next/auth/return-to";
import { LoginForm } from "@/app/components/auth/LoginForm";

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

  const customer = await getCustomerOrNull();
  if (customer) redirect(await withLocalePrefix(returnTo ?? "/account", locale));

  return (
    <AuthCard
      title={t("title")}
      description={t("description")}
      banner={
        needsVerify ? (
          <FormSuccess>{t("verifyBanner")}</FormSuccess>
        ) : verifySent ? (
          <FormSuccess>{t("verifySentBanner")}</FormSuccess>
        ) : justReset ? (
          <FormSuccess>{t("resetBanner")}</FormSuccess>
        ) : undefined
      }
    >
      <LoginForm returnTo={returnTo} />
    </AuthCard>
  );
}
