import { getTranslations } from "next-intl/server";
import { Link } from "@/app/components/LocaleLink";
import { getStoreAvailabilityEnabled } from "@skelet/storefront-next/config/read";

export async function StoresLink() {
  if (!(await getStoreAvailabilityEnabled())) return null;
  const t = await getTranslations("stores");
  return <Link className="text-sm underline" href="/stores">{t("title")}</Link>;
}
