import { redirect } from "next/navigation";
import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";
import { withLocalePrefix } from "@/app/lib/locale/path";

import { AuthCard } from "@/app/components/account/AccountUI";
import { getCustomerOrNull } from "@/app/lib/customer/read";
import { safeReturnTo } from "@/app/lib/auth/return-to";
import { RegisterForm } from "@/app/components/auth/RegisterForm";

export async function generateMetadata({
  params,
}: PageProps<"/[channel]/[locale]/account/register">): Promise<Metadata> {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "auth.register" });
  return { title: t("title") };
}

export default async function RegisterPage({
  params,
  searchParams,
}: PageProps<"/[channel]/[locale]/account/register">) {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "auth.register" });
  const sp = await searchParams;
  const returnTo = safeReturnTo(sp.returnTo);
  const refRaw = Array.isArray(sp.ref) ? sp.ref[0] : sp.ref;
  const referralCode = refRaw?.trim() || null;

  const customer = await getCustomerOrNull();
  if (customer) redirect(await withLocalePrefix(returnTo ?? "/account", locale));

  return (
    <AuthCard
      title={t("title")}
      description={t("description")}
    >
      <RegisterForm returnTo={returnTo} referralCode={referralCode} />
    </AuthCard>
  );
}
