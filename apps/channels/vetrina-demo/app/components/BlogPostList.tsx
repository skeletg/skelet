// Shared blog post list - one paper card per post

import { Link } from "@/app/components/LocaleLink";

import { BlogTaxonomyChip } from "./BlogTaxonomyChip";
import {
  blogAuthorHref,
  blogCategoryHref,
  blogTagHref,
  getBlogTaxonomy,
  type BlogPostSummary,
} from "@/app/lib/blog/read";
import { displayName } from "@/app/lib/display";
import { blogPostHref } from "@/app/lib/entity-href";
import { formatDate } from "@/app/lib/format";
import { renderJsonText } from "@/app/lib/json-text";

export async function BlogPostList({
  posts,
  className,
}: {
  posts: BlogPostSummary[];
  className?: string;
}) {
  const { categoryNames, tagNames } = await getBlogTaxonomy();

  return (
    <ul className={`flex flex-col gap-4 ${className ?? "mt-8"}`}>
      {posts.map((p) => {
        const title = displayName(p.content?.title, p.slug);
        const excerpt = renderJsonText(p.content?.excerpt);
        return (
          <li key={p.id} className="glass rounded-xl p-6">
            <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1 text-xs text-muted-foreground">
              {p.publishedAt && (
                <time dateTime={p.publishedAt}>{formatDate(p.publishedAt)}</time>
              )}
              {p.author && (
                <>
                  <span aria-hidden="true">·</span>
                  <Link
                    href={blogAuthorHref(p.author)}
                    className="hover:text-foreground hover:underline"
                  >
                    {p.author.displayName}
                  </Link>
                </>
              )}
              {p.readingTimeMinutes && (
                <>
                  <span aria-hidden="true">·</span>
                  <span>{p.readingTimeMinutes} min read</span>
                </>
              )}
            </div>

            <h2 className="mt-1 text-xl font-medium tracking-tight">
              <Link href={blogPostHref(p)} className="hover:underline">
                {title}
              </Link>
            </h2>

            {excerpt && (
              <p className="mt-2 text-sm leading-relaxed text-muted-foreground">
                {excerpt}
              </p>
            )}

            {(p.categories.length > 0 || p.tags.length > 0) && (
              <ul className="mt-4 flex flex-wrap gap-1.5 text-xs">
                {p.categories.map((c) => (
                  <li key={c.id}>
                    <BlogTaxonomyChip href={blogCategoryHref(c)}>
                      {displayName(
                        c.content?.name ?? categoryNames.get(c.slug),
                        c.slug,
                      )}
                    </BlogTaxonomyChip>
                  </li>
                ))}
                {p.tags.map((t) => (
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
            )}
          </li>
        );
      })}
    </ul>
  );
}
