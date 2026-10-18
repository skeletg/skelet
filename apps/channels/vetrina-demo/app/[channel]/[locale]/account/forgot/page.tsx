import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";

import { AuthCard } from "@/app/components/account/AccountUI";
import { ForgotForm } from "@/app/components/auth/ForgotForm";

export async function generateMetadata({
  params,
}: PageProps<"/[channel]/[locale]/account/forgot">): Promise<Metadata> {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "auth.forgot" });
  return { title: t("title") };
}

export default async function ForgotPasswordPage({
  params,
}: PageProps<"/[channel]/[locale]/account/forgot">) {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "auth.forgot" });
  return (
    <AuthCard title={t("title")} description={t("description")}>
      <ForgotForm />
    </AuthCard>
  );
}
