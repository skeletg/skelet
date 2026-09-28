import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";

import { AuthCard } from "@/app/components/account/AccountUI";
import { ResetForm } from "./ResetForm";

export async function generateMetadata({
  params,
}: PageProps<"/[channel]/[locale]/account/reset/[token]">): Promise<Metadata> {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "auth.reset" });
  return {
    title: t("title"),
    // The token-bearing URL must never end up in a search index
    robots: { index: false, follow: false },
  };
}

export default async function ResetPasswordPage(
  props: PageProps<"/[channel]/[locale]/account/reset/[token]">,
) {
  const { token, locale } = await props.params;
  const t = await getTranslations({ locale, namespace: "auth.reset" });
  return (
    // lens-block keeps the whole reset surface out of session replay + DOM snapshots
    <div className="lens-block">
      <AuthCard
        title={t("title")}
        description={t("description")}
      >
        <ResetForm token={token} />
      </AuthCard>
    </div>
  );
}
