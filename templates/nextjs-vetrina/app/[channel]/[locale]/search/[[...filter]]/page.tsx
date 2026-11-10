// Full product search results - the "view all" target from the header search modal

import { Link } from "@/app/components/LocaleLink";
import { getLocale, getTranslations } from "next-intl/server";
import { withLocalePrefix } from "@skelet/storefront-next/locale/path";
import type { Metadata } from "next";
import { headers } from "next/headers";
import { redirect } from "next/navigation";
import { userAgent } from "next/server";

import { ProductListShell } from "@/app/components/ProductListShell";
import { SearchField } from "@/app/components/SearchField";
import { SearchOverview } from "@/app/components/search-overview/SearchOverview";
import {
  parsePageFromSegments, parseFilterSegments, parseSortFromSegments } from "@skelet/storefront-core/filter-path";
import { searchProducts } from "@skelet/storefront-next/search/read";
import { searchedOnSite } from "@skelet/storefront-core/search/overview";

export const dynamic = "force-dynamic";

type RawSearchParams = Record<string, string | string[] | undefined>;

export async function generateMetadata({
  params: routeParams,
  searchParams,
}: PageProps<"/[channel]/[locale]/search/[[...filter]]">): Promise<Metadata> {
  const { locale } = await routeParams;
  const t = await getTranslations({ locale, namespace: "search" });
  const params = (await searchParams) as RawSearchParams;
  const q = pickQuery(params);
  return {
    title: q ? t("resultsTitle", { query: q }) : t("title"),
    robots: { index: false, follow: false },
  };
}

export default async function SearchPage(
  props: PageProps<"/[channel]/[locale]/search/[[...filter]]">,
) {
  const params = (await props.searchParams) as RawSearchParams;
  const { filter, locale } = await props.params;
  const t = await getTranslations({ locale, namespace: "search" });
  const q = pickQuery(params);
  const pathFilters = filter ? parseFilterSegments(filter) : [];
  const sortSlug = filter ? parseSortFromSegments(filter) : "";

  if (!q) {
    return (
      <main className="mx-auto w-full max-w-3xl px-6 pt-6 pb-10">
        <h1 className="text-2xl font-extrabold tracking-tight">{t("title")}</h1>
        <SearchField initialQuery="" autoFocus />
        <p className="mt-6 text-sm text-muted-foreground">
          {t.rich("prompt", {
            link: (chunks) => (
              <Link href="/blog" className="underline">
                {chunks}
              </Link>
            ),
          })}
        </p>
      </main>
    );
  }

  const probe = await searchProducts(q, { limit: 1 });
  if (probe.redirectUrl) redirect(await withLocalePrefix(probe.redirectUrl, await getLocale()));
  const page = parsePageFromSegments(filter ?? []);
  // the backend decides which listings get an overview; the page keeps crawlers and direct landings from writing one
  const requestHeaders = await headers();
  const overview = !userAgent({ headers: requestHeaders }).isBot && searchedOnSite(requestHeaders);

  return (
    <ProductListShell
      query={q}
      searchParams={params}
      pathFilters={pathFilters}
      pathSort={sortSlug}
      pathPage={page}
      basePath="/search"
      emptyText={t("empty", { query: q })}
      overview={overview ? <SearchOverview query={q} /> : undefined}
      header={
        <>
          <h1 className="text-balance text-3xl font-extrabold leading-tight tracking-tight md:text-5xl md:leading-[1.08]">
            {t.rich("resultsFor", {
              query: q,
              q: (chunks) => <span className="text-muted-foreground">{chunks}</span>,
            })}
          </h1>
          <SearchField initialQuery={q} />
        </>
      }
    />
  );
}

function pickQuery(sp: RawSearchParams): string {
  const raw = sp.q;
  if (typeof raw === "string") return raw.trim();
  if (Array.isArray(raw)) return (raw[0] ?? "").trim();
  return "";
}
