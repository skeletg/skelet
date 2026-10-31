import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";
import { readPublicStores } from "@/app/lib/stores/read";
import { StoresPage } from "@/app/components/stores/StoresPage";

export const dynamic = "force-dynamic";
export async function generateMetadata() {
  const t = await getTranslations("stores");
  return { title: t("title"), robots: { index: false, follow: true } };
}
// One read answers both questions the page has: whether stores are on, and whether product stock can be checked
export default async function Page() {
  const [gate, t] = await Promise.all([readPublicStores({ limit: 1 }), getTranslations("stores")]);
  if (!gate.enabled) notFound();
  return <div className="mx-auto max-w-4xl px-4 py-10"><h1 className="mb-3 text-3xl font-semibold">{t("title")}</h1><p className="mb-6 text-muted-foreground">{t("intro")}</p><StoresPage stockChecking={gate.stockCheckingAvailable} /></div>;
}
