import { Link } from "@/app/components/LocaleLink";
import { SparklesIcon } from "lucide-react";
import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";

import { listCollections } from "@skelet/storefront-next/catalog/read";
import { displayName, MISSING_NAME } from "@skelet/storefront-next/display";
import { collectionHref } from "@skelet/storefront-next/entity-href";
import { renderJsonText } from "@skelet/storefront-next/json-text";
import { localizedAlternates } from "@skelet/storefront-next/locale/alternates";

export async function generateMetadata({
  params,
}: PageProps<"/[channel]/[locale]/collections">): Promise<Metadata> {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "listing" });
  return {
    title: t("collections"),
    alternates: await localizedAlternates("/collections"),
  };
}

export default async function CollectionsPage({
  params,
}: PageProps<"/[channel]/[locale]/collections">) {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "listing" });
  const { collections } = await listCollections();

  return (
    <main className="mx-auto w-full max-w-8xl px-6 md:px-10 pt-6 pb-14">
      <p className="text-xs uppercase tracking-wider text-muted-foreground">
        {t("curated")}
      </p>
      <h1 className="mt-1 text-3xl font-medium tracking-tight">
        {t("collections")}
      </h1>

      {collections.length === 0 ? (
        <p className="mt-8 text-sm text-muted-foreground">
          {t("noCollections")}
        </p>
      ) : (
        <ul className="mt-8 grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {collections.map((c) => {
            const description = renderJsonText(c.description);
            return (
              <li key={c.id}>
                <Link
                  href={collectionHref(c)}
                  className="glass glass-hover group flex h-full flex-col gap-3 rounded-2xl p-5 transition"
                >
                  <div className="flex items-center gap-3">
                    {c.coverImage ? (
                      // eslint-disable-next-line @next/next/no-img-element
                      <img
                        src={c.coverImage.url}
                        alt={c.coverImage.altText || c.name || MISSING_NAME}
                        className="h-12 w-12 rounded-md object-cover"
                      />
                    ) : (
                      <span className="flex h-12 w-12 shrink-0 items-center justify-center rounded-md bg-accent">
                        <SparklesIcon
                          className="h-5 w-5 text-foreground"
                          aria-hidden="true"
                        />
                      </span>
                    )}
                    <h2 className="text-base font-medium leading-tight group-hover:underline">
                      {displayName(c.name, c.slug)}
                    </h2>
                  </div>
                  {description && (
                    <p className="line-clamp-3 text-sm text-muted-foreground">
                      {description}
                    </p>
                  )}
                </Link>
              </li>
            );
          })}
        </ul>
      )}

    </main>
  );
}
