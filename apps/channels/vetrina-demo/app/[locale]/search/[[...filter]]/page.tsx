// Full product search results - the "view all" target from the header search modal

import { Link } from "@/app/components/LocaleLink";
import { getLocale } from "next-intl/server";
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
  searchParams,
}: {
  searchParams: Promise<RawSearchParams>;
}): Promise<Metadata> {
  const params = await searchParams;
  const q = pickQuery(params);
  return {
    title: q ? `Search results - "${q}"` : "Search",
    robots: { index: false, follow: false },
  };
}

export default async function SearchPage(
  props: PageProps<"/[locale]/search/[[...filter]]">,
) {
  const params = (await props.searchParams) as RawSearchParams;
  const { filter } = await props.params;
  const q = pickQuery(params);
  const pathFilters = filter ? parseFilterSegments(filter) : [];
  const sortSlug = filter ? parseSortFromSegments(filter) : "";

  if (!q) {
    return (
      <main className="mx-auto w-full max-w-3xl px-6 pt-6 pb-10">
        <h1 className="text-2xl font-extrabold tracking-tight">Search</h1>
        <SearchForm initialQuery="" autoFocus />
        <p className="mt-6 text-sm text-muted-foreground">
          Type a query above to search products. Looking for posts? Try
          the <Link href="/blog" className="underline">blog</Link>.
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
      emptyText={`No products matched "${q}". Try a different query or refine the filters.`}
      header={
        <>
          <h1 className="text-2xl font-extrabold tracking-tight">
            Results for <span className="text-muted-foreground">“{q}”</span>
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
  return (
    <form action="/search" method="GET" className="mt-4 flex gap-2">
      <input
        type="search"
        name="q"
        defaultValue={initialQuery}
        autoFocus={autoFocus}
        placeholder="Search products…"
        className="flex-1 rounded-md border border-border bg-transparent px-4 py-2 text-sm outline-none focus:border-ring"
        autoComplete="off"
      />
      <button
        type="submit"
        className="rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90"
      >
        Search
      </button>
    </form>
  );
}
