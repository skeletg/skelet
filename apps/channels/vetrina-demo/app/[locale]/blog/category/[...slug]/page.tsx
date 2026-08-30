// Category archive - every published post filed under one category

import type { Metadata } from "next";
import { localizedAlternates } from "@/app/lib/locale/alternates";
import { notFound, permanentRedirect } from "next/navigation";
import { BlogArchive } from "@/app/components/BlogArchive";
import {
  BLOG_ARCHIVE_PAGE_SIZE,
  blogCategoryHref,
  blogTaxonomyRedirect,
  getBlogCategory,
  listBlogPostsBy,
  parseBlogPage,
} from "@/app/lib/blog/read";
import { displayName } from "@/app/lib/display";
import { renderJsonText } from "@/app/[locale]/products/[id]/parts";

// The requested path as the backend stores it (no leading slash) plus the slug to look
// the category up by
function readParams(slug: string[]): { path: string; slug: string } {
  return {
    path: ["blog", "category", ...slug].join("/"),
    slug: slug[slug.length - 1],
  };
}

export async function generateMetadata(
  props: PageProps<"/[locale]/blog/category/[...slug]">,
): Promise<Metadata> {
  const { slug: segments } = await props.params;
  const { slug } = readParams(segments);
  const category = await getBlogCategory(slug);
  if (!category) return { title: "Category not found" };
  const name = displayName(category.content?.name, category.slug);
  const page = parseBlogPage(await props.searchParams);
  const href = blogCategoryHref(category);
  return {
    title:
      category.content?.metaTitle ??
      (page > 1 ? `${name} - page ${page}` : name),
    description:
      category.content?.metaDescription ??
      renderJsonText(category.content?.description) ??
      `Posts filed under ${name}.`,
    // Editorial noindex flag, same treatment the post page gives it
    robots: category.seoNoindex ? { index: false } : undefined,
    alternates: await localizedAlternates(page > 1 ? `${href}?page=${page}` : href),
  };
}

export default async function BlogCategoryPage(
  props: PageProps<"/[locale]/blog/category/[...slug]">,
) {
  const { slug: segments } = await props.params;
  const { path, slug } = readParams(segments);
  const page = parseBlogPage(await props.searchParams);

  const category = await getBlogCategory(slug);
  if (!category) {
    const to = await blogTaxonomyRedirect(path);
    if (to) permanentRedirect(to);
    notFound();
  }
  // Reached via a stale or hand-shortened ancestor chain - send the crawler to the one
  // canonical path
  const href = blogCategoryHref(category);
  if (category.urlPath !== path) permanentRedirect(href);

  const { posts, hasMore } = await listBlogPostsBy(
    { categorySlug: category.slug },
    {
      limit: BLOG_ARCHIVE_PAGE_SIZE,
      offset: (page - 1) * BLOG_ARCHIVE_PAGE_SIZE,
    },
  );
  if (page > 1 && posts.length === 0) notFound();

  return (
    <BlogArchive
      eyebrow="Category"
      title={displayName(category.content?.name, category.slug)}
      description={renderJsonText(category.content?.description)}
      href={href}
      posts={posts}
      page={page}
      hasMore={hasMore}
    />
  );
}
