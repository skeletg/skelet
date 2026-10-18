import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";

import { AuthCard } from "@/app/components/account/AccountUI";
import { Link } from "@/app/components/LocaleLink";
import { TrackOrderForm } from "./TrackOrderForm";

export async function generateMetadata({ params }: PageProps<"/[channel]/[locale]/track-order">): Promise<Metadata> {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "trackOrder" });
  return { title: t("title"), robots: { index: false } };
}

const first = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v);

// ?number=&email= prefill it, so an order email can link straight here
export default async function TrackOrderPage({ params, searchParams }: PageProps<"/[channel]/[locale]/track-order">) {
  const { locale } = await params;
  const sp = await searchParams;
  const t = await getTranslations({ locale, namespace: "trackOrder" });
  return (
    <main className="mx-auto w-full max-w-md px-6 pt-10 pb-20">
      <AuthCard title={t("title")} description={t("description")}>
        <TrackOrderForm number={first(sp.number)} email={first(sp.email)} />
        <p className="mt-6 text-center text-sm text-muted-foreground">
          {t("haveAccount")}{" "}
          <Link href="/account/orders" className="text-foreground underline underline-offset-2">
            {t("signInHint")}
          </Link>
        </p>
      </AuthCard>
    </main>
  );
}
