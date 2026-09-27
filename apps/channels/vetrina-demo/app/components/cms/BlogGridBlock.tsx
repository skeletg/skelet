import { Link } from "@/app/components/LocaleLink";
import { NewspaperIcon } from "lucide-react";

import type { Block } from "./blocks";
import { readNumber, readString } from "./blocks";
import { CmsSection } from "./Section";
import { listBlogPosts } from "@/app/lib/blog/read";
import { displayName } from "@/app/lib/display";
import { formatDate } from "@/app/lib/format";
import { renderJsonText } from "@/app/lib/json-text";

export async function BlogGridBlock({ block }: { block: Block }) {
  const heading = readString(block.props, "heading");
  const limit = readNumber(block.props, "limit") ?? 3;
  const { posts } = await listBlogPosts({ limit });

  if (posts.length === 0) return null;

  return (
    <CmsSection heading={heading} href="/blog" hrefLabel="All posts">
      <div className="grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-3">
        {posts.map((p) => {
          const title = displayName(p.content?.title, p.slug);
          const excerpt = renderJsonText(p.content?.excerpt);
          return (
            <Link
              key={p.id}
              href={`/blog/${p.slug}`}
              className="glass glass-hover group flex flex-col rounded-lg p-5"
            >
              <NewspaperIcon
                className="h-5 w-5 text-signal"
                aria-hidden="true"
              />
              <h3 className="mt-3 text-base font-medium group-hover:underline">
                {title}
              </h3>
              {excerpt && (
                <p className="mt-2 line-clamp-3 text-sm leading-relaxed text-muted-foreground">
                  {excerpt}
                </p>
              )}
              <p className="mt-4 text-xs text-muted-foreground">
                {p.publishedAt && (
                  <time dateTime={p.publishedAt}>
                    {formatDate(p.publishedAt)}
                  </time>
                )}
                {p.author && (
                  <>
                    {" · "}
                    <span>{p.author.displayName}</span>
                  </>
                )}
              </p>
            </Link>
          );
        })}
      </div>
    </CmsSection>
  );
}
