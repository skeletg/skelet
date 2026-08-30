// Shared chrome for the three blog taxonomy archives (/blog/author, /blog/category,
// /blog/tag)

import { Link } from "@/app/components/LocaleLink";

import { BlogPostList } from "./BlogPostList";
import { Breadcrumbs } from "./Breadcrumbs";
import type { BlogPostSummary } from "@/app/lib/blog/read";

export function BlogArchive({
  eyebrow,
  title,
  description,
  href,
  posts,
  page,
  hasMore,
}: {
  eyebrow: string;
  title: string;
  description: string | null;
  href: string;
  posts: BlogPostSummary[];
  page: number;
  hasMore: boolean;
}) {
  return (
    <main className="mx-auto w-full max-w-3xl px-6 pt-6 pb-14">
      <Breadcrumbs
        items={[
          { name: "Home", href: "/" },
          { name: "Blog", href: "/blog" },
          { name: title, href },
        ]}
      />

      <header className="glass mt-6 rounded-xl p-6">
        <p className="text-xs uppercase tracking-wider text-muted-foreground">
          {eyebrow}
        </p>
        <h1 className="mt-1 text-3xl font-extrabold tracking-tight">{title}</h1>
        {description && (
          <p className="mt-3 text-base leading-relaxed text-muted-foreground">
            {description}
          </p>
        )}
      </header>

      {posts.length === 0 ? (
        <p className="mt-8 text-sm text-muted-foreground">
          No posts here yet.{" "}
          <Link href="/blog" className="underline">
            Back to the blog →
          </Link>
        </p>
      ) : (
        <BlogPostList posts={posts} />
      )}

      <BlogArchivePager href={href} page={page} hasMore={hasMore} />

    </main>
  );
}

function BlogArchivePager({
  href,
  page,
  hasMore,
}: {
  href: string;
  page: number;
  hasMore: boolean;
}) {
  if (page === 1 && !hasMore) return null;
  const pageHref = (n: number) => (n === 1 ? href : `${href}?page=${n}`);
  const link =
    "rounded-md bg-muted px-4 py-2 text-sm text-muted-foreground hover:bg-accent hover:text-foreground";
  return (
    <nav
      aria-label="Pagination"
      className="mt-10 flex items-center justify-between gap-3"
    >
      {page > 1 ? (
        <Link href={pageHref(page - 1)} rel="prev" className={link}>
          ← Newer posts
        </Link>
      ) : (
        <span />
      )}
      {hasMore ? (
        <Link href={pageHref(page + 1)} rel="next" className={link}>
          Older posts →
        </Link>
      ) : (
        <span />
      )}
    </nav>
  );
}
