// Full product search results - the "view all" target from the header search modal

import { Link } from "@/app/components/LocaleLink";
import { useTranslations } from "next-intl";
import { getLocale, getTranslations } from "next-intl/server";
import { withLocalePrefix } from "@/app/lib/locale/path";
import type { Metadata } from "next";
import { redirect } from "next/navigation";

import { ProductListShell } from "@/app/components/ProductListShell";
import {
  parsePageFromSegments, parseFilterSegments, parseSortFromSegments } from "@/app/lib/filter-path";
import { searchProducts } from "@/app/lib/search/read";

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
        <SearchForm initialQuery="" autoFocus />
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

  return (
    <ProductListShell
      query={q}
      searchParams={params}
      pathFilters={pathFilters}
      pathSort={sortSlug}
      pathPage={parsePageFromSegments(filter ?? [])}
      basePath="/search"
      emptyText={t("empty", { query: q })}
      header={
        <>
          <h1 className="text-2xl font-extrabold tracking-tight">
            {t.rich("resultsFor", {
              query: q,
              q: (chunks) => <span className="text-muted-foreground">{chunks}</span>,
            })}
          </h1>
          <SearchForm initialQuery={q} />
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

function SearchForm({
  initialQuery,
  autoFocus,
}: {
  initialQuery: string;
  autoFocus?: boolean;
}) {
  const t = useTranslations("search");
  return (
    <form action="/search" method="GET" className="mt-4 flex gap-2">
      <input
        type="search"
        name="q"
        defaultValue={initialQuery}
        autoFocus={autoFocus}
        placeholder={t("placeholder")}
        className="flex-1 rounded-md border border-border bg-transparent px-4 py-2 text-sm outline-none focus:border-ring"
        autoComplete="off"
      />
      <button
        type="submit"
        className="rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90"
      >
        {t("submit")}
      </button>
    </form>
  );
}
