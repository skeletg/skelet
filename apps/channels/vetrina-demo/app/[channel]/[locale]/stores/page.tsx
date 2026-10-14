import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";
import { readPublicStores } from "@/app/lib/stores/read";
import { StoreFinder } from "@/app/components/stores/StoreFinder";

export const dynamic = "force-dynamic";
export async function generateMetadata() {
  const t = await getTranslations("stores");
  return { title: t("title"), robots: { index: false, follow: true } };
}
export default async function StoresPage() {
  const [gate, t] = await Promise.all([readPublicStores({ limit: 1 }), getTranslations("stores")]);
  if (!gate.enabled) notFound();
  return <div className="mx-auto max-w-4xl px-4 py-10"><h1 className="mb-3 text-3xl font-semibold">{t("title")}</h1><p className="mb-6 text-muted-foreground">{t("intro")}</p><StoreFinder allowProductSearch /></div>;
}
