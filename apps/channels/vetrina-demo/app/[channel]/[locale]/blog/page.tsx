import { Link } from "@/app/components/LocaleLink";
import { useTranslations } from "next-intl";
import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";

import { BlogPostList } from "@/app/components/BlogPostList";
import { BlogTaxonomyChip } from "@/app/components/BlogTaxonomyChip";
import {
  blogCategoryHref,
  blogTagHref,
  getBlogTaxonomy,
  listBlogPosts,
} from "@/app/lib/blog/read";
import { displayName } from "@/app/lib/display";
import { formatDate } from "@/app/lib/format";
import { searchBlogPosts } from "@/app/lib/search/read";

type SearchParams = {
  q?: string;
};

export async function generateMetadata({
  params: routeParams,
  searchParams,
}: {
  params: Promise<{ locale: string }>;
  searchParams: Promise<SearchParams>;
}): Promise<Metadata> {
  const { locale } = await routeParams;
  const t = await getTranslations({ locale, namespace: "blog" });
  const params = await searchParams;
  const q = (params.q ?? "").trim();
  return {
    title: q ? t("searchTitle", { query: q }) : t("title"),
    robots: q ? { index: false, follow: false } : undefined,
  };
}

export default async function BlogIndexPage({
  params: routeParams,
  searchParams,
}: {
  params: Promise<{ locale: string }>;
  searchParams: Promise<SearchParams>;
}) {
  const { locale } = await routeParams;
  const t = await getTranslations({ locale, namespace: "blog" });
  const params = await searchParams;
  const q = (params.q ?? "").trim();

  return (
    <main className="mx-auto w-full max-w-3xl px-6 pt-6 pb-12">
      <p className="text-xs uppercase tracking-wider text-muted-foreground">
        {t("journal")}
      </p>
      <h1 className="mt-1 text-3xl font-extrabold tracking-tight">{t("title")}</h1>

      <SearchForm initialQuery={q} />

      {q ? (
        <SearchResults query={q} />
      ) : (
        <>
          <TaxonomyBar />
          <RecentPosts />
        </>
      )}
    </main>
  );
}

async function TaxonomyBar() {
  const [{ categories, tags }, t] = await Promise.all([
    getBlogTaxonomy(),
    getTranslations("blog"),
  ]);
  if (categories.length === 0 && tags.length === 0) return null;
  return (
    <nav aria-label={t("taxonomy")} className="glass mt-6 rounded-xl p-5">
      {categories.length > 0 && (
        <div className="flex flex-wrap items-baseline gap-x-3 gap-y-2">
          <span className="text-xs uppercase tracking-wider text-muted-foreground">
            {t("categories")}
          </span>
          <ul className="flex flex-wrap gap-1.5 text-xs">
            {categories.map((c) => (
              <li key={c.id}>
                <BlogTaxonomyChip href={blogCategoryHref(c)}>
                  {displayName(c.content?.name, c.slug)}
                </BlogTaxonomyChip>
              </li>
            ))}
          </ul>
        </div>
      )}
      {tags.length > 0 && (
        <div
          className={`flex flex-wrap items-baseline gap-x-3 gap-y-2 ${categories.length > 0 ? "mt-3" : ""}`}
        >
          <span className="text-xs uppercase tracking-wider text-muted-foreground">
            {t("tags")}
          </span>
          <ul className="flex flex-wrap gap-1.5 text-xs">
            {tags.map((t) => (
              <li key={t.id}>
                <BlogTaxonomyChip href={blogTagHref(t)}>
                  #{displayName(t.content?.name, t.slug)}
                </BlogTaxonomyChip>
              </li>
            ))}
          </ul>
        </div>
      )}
    </nav>
  );
}

async function RecentPosts() {
  const [{ posts }, t] = await Promise.all([
    listBlogPosts({ limit: 20 }),
    getTranslations("blog"),
  ]);
  if (posts.length === 0) {
    return (
      <p className="mt-8 text-sm text-muted-foreground">
        {t("noPosts")}
      </p>
    );
  }
  return (
    <>
      <BlogPostList posts={posts} />
    </>
  );
}

async function SearchResults({ query }: { query: string }) {
  const [{ items, totalCount }, t] = await Promise.all([
    searchBlogPosts(query, { limit: 30 }),
    getTranslations("blog"),
  ]);
  if (totalCount === 0) {
    return (
      <p className="mt-8 text-sm text-muted-foreground">
        {t("noMatch", { query })}
      </p>
    );
  }
  return (
    <>
      <p className="mt-6 text-xs uppercase tracking-wider text-muted-foreground">
        {totalCount} result{totalCount === 1 ? "" : "s"}
      </p>
      <ul className="mt-4 divide-y divide-border border-y border-border">
        {items.map((p) => (
          <li key={p.id} className="py-6">
            <Link href={`/blog/${p.slug}`} className="group block">
              <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1 text-xs text-muted-foreground">
                {p.publishedAt && (
                  <time dateTime={p.publishedAt}>
                    {formatDate(p.publishedAt)}
                  </time>
                )}
                {p.authorName && (
                  <>
                    <span>·</span>
                    <span>{p.authorName}</span>
                  </>
                )}
              </div>
              <h2 className="mt-1 text-xl font-medium tracking-tight group-hover:underline">
                {displayName(p.title, p.slug)}
              </h2>
              {p.excerpt && (
                <p className="mt-2 line-clamp-3 text-sm leading-relaxed text-muted-foreground">
                  {p.excerpt}
                </p>
              )}
            </Link>
          </li>
        ))}
      </ul>
    </>
  );
}

function SearchForm({ initialQuery }: { initialQuery: string }) {
  const t = useTranslations("blog");
  const tc = useTranslations("common");
  return (
    <form action="/blog" method="GET" className="mt-6 flex gap-2">
      <input
        type="search"
        name="q"
        defaultValue={initialQuery}
        placeholder={t("searchPlaceholder")}
        aria-label={t("searchLabel")}
        className="flex-1 rounded-md border border-border bg-transparent px-4 py-2 text-sm outline-none focus:border-foreground/40"
        autoComplete="off"
      />
      <button
        type="submit"
        className="rounded-md bg-foreground px-4 py-2 text-sm font-medium text-background"
      >
        {t("search")}
      </button>
      {initialQuery && (
        <Link
          href="/blog"
          className="flex items-center px-3 text-xs text-muted-foreground hover:text-foreground"
        >
          {tc("clear")}
        </Link>
      )}
    </form>
  );
}
