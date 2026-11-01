import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";
import { getSiteUrl } from "@skelet/storefront-next/channel/site";
import { localizedAlternates } from "@skelet/storefront-next/locale/alternates";
import { Link } from "@/app/components/LocaleLink";
import { notFound } from "next/navigation";

import { Breadcrumbs } from "@/app/components/Breadcrumbs";
import { BlogTaxonomyChip } from "@/app/components/BlogTaxonomyChip";
import { JsonLd } from "@/app/components/JsonLd";
import {
  blogAuthorHref,
  blogCategoryHref,
  blogTagHref,
  getBlogPost,
  getBlogTaxonomy,
} from "@skelet/storefront-next/blog/read";
import { displayName } from "@skelet/storefront-core/display";
import { formatDate } from "@skelet/storefront-core/format";
import { blogPostHref } from "@skelet/storefront-core/entity-href";
import { absUrl } from "@skelet/storefront-next/sitemap/read";
import { blogPostingJsonLd, breadcrumbJsonLd } from "@skelet/storefront-core/json-ld";
import { canonicalPathMapper } from "@skelet/storefront-next/locale/alternates";
import { LexicalRichText } from "@/app/lib/lexical-render";
import { lexicalHasContent, lexicalPlainText, lexicalSummary } from "@skelet/storefront-core/lexical";

export async function generateMetadata(
  props: PageProps<"/[channel]/[locale]/blog/[slug]">,
): Promise<Metadata> {
  const { slug, locale } = await props.params;
  const { post } = await getBlogPost(slug);
  if (!post) {
    const t = await getTranslations({ locale, namespace: "blog" });
    return { title: t("postNotFound") };
  }
  const title = displayName(post.content?.metaTitle ?? post.content?.title, post.slug);
  const description =
    post.content?.metaDescription ||
    lexicalSummary(post.content?.excerpt) ||
    lexicalSummary(post.content?.body) ||
    undefined;
  const href = blogPostHref(post);
  return {
    title,
    description,
    // Respect the editorial noindex flag - the API exposes it so
    // unfinished posts can be linked without polluting search
    robots: post.seoNoindex ? { index: false } : undefined,
    alternates: await localizedAlternates(href),
    openGraph: {
      type: "article",
      title,
      description,
      url: absUrl(href, await getSiteUrl()),
      publishedTime: post.publishedAt ?? undefined,
    },
    twitter: { card: "summary", title, description },
  };
}

export default async function BlogPostPage(
  props: PageProps<"/[channel]/[locale]/blog/[slug]">,
) {
  const { slug } = await props.params;
  const { post } = await getBlogPost(slug);
  if (!post) notFound();

  const { categoryNames, tagNames } = await getBlogTaxonomy();

  const title = displayName(post.content?.title, post.slug);
  const excerpt = lexicalPlainText(post.content?.excerpt) || null;
  // hasBody must count all blocks, not just text - a body composed entirely of custom
  // blocks (FAQ, image, structured) has no text leaves but is still real content
  const hasBody = lexicalHasContent(post.content?.body);

  const href = `/blog/${post.slug}`;
  const [tb, tc] = await Promise.all([getTranslations("blog"), getTranslations("common")]);
  const breadcrumbItems = [
    { name: tc("home"), href: "/" },
    { name: tb("title"), href: "/blog" },
    { name: title, href },
  ];
  const [canonical, base] = await Promise.all([canonicalPathMapper(), getSiteUrl()]);

  return (
    <main className="mx-auto w-full max-w-3xl px-6 pt-6 pb-12">
      {!post.seoNoindex && (
        <>
          <JsonLd data={blogPostingJsonLd(post, canonical(href), base)} />
          <JsonLd
            data={breadcrumbJsonLd(
              breadcrumbItems.map((it) => ({ ...it, href: canonical(it.href) })),
              base,
            )}
          />
        </>
      )}
      <Breadcrumbs items={breadcrumbItems} />

      <article className="mt-6">
        <header className="border-b border-border pb-6">
          <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1 text-xs text-muted-foreground">
            {post.publishedAt && (
              <time dateTime={post.publishedAt}>
                {formatDate(post.publishedAt)}
              </time>
            )}
            {post.author && (
              <>
                <span aria-hidden="true">·</span>
                <Link
                  href={blogAuthorHref(post.author)}
                  className="hover:text-foreground hover:underline"
                  rel="author"
                >
                  {post.author.displayName}
                </Link>
              </>
            )}
            {post.readingTimeMinutes && (
              <>
                <span>·</span>
                <span>{post.readingTimeMinutes} min read</span>
              </>
            )}
          </div>
          <h1 className="mt-2 text-3xl font-extrabold tracking-tight">
            {title}
          </h1>
          {excerpt && (
            <p className="mt-3 text-base leading-relaxed text-muted-foreground">
              {excerpt}
            </p>
          )}
        </header>

        {hasBody ? (
          <div className="mt-8 space-y-4 text-base leading-relaxed">
            <LexicalRichText value={post.content?.body} />
          </div>
        ) : (
          <p className="mt-8 text-sm text-muted-foreground">
            {tb("noBody")}
          </p>
        )}

        {(post.categories.length > 0 || post.tags.length > 0) && (
          <footer className="mt-12 border-t border-border pt-6">
            {post.categories.length > 0 && (
              <div className="flex flex-wrap items-baseline gap-x-3 gap-y-2">
                <span className="text-xs text-muted-foreground">{tb("categories")}</span>
                <ul className="flex flex-wrap gap-1.5 text-xs">
                  {post.categories.map((c) => (
                    <li key={c.id}>
                      <BlogTaxonomyChip href={blogCategoryHref(c)}>
                        {displayName(
                          c.content?.name ?? categoryNames.get(c.slug),
                          c.slug,
                        )}
                      </BlogTaxonomyChip>
                    </li>
                  ))}
                </ul>
              </div>
            )}
            {post.tags.length > 0 && (
              <div className="mt-3 flex flex-wrap items-baseline gap-x-3 gap-y-2">
                <span className="text-xs text-muted-foreground">{tb("tags")}</span>
                <ul className="flex flex-wrap gap-1.5 text-xs">
                  {post.tags.map((t) => (
                    <li key={t.id}>
                      <BlogTaxonomyChip href={blogTagHref(t)}>
                        #
                        {displayName(
                          t.content?.name ?? tagNames.get(t.slug),
                          t.slug,
                        )}
                      </BlogTaxonomyChip>
                    </li>
                  ))}
                </ul>
              </div>
            )}
          </footer>
        )}
      </article>

    </main>
  );
}
