// Author archive - every published post by one writer

import type { Metadata } from "next";
import { localizedAlternates } from "@/app/lib/locale/alternates";
import { notFound, permanentRedirect } from "next/navigation";
import { BlogArchive } from "@/app/components/BlogArchive";
import {
  BLOG_ARCHIVE_PAGE_SIZE,
  blogAuthorHref,
  blogTaxonomyRedirect,
  getBlogAuthor,
  listBlogPostsBy,
  parseBlogPage,
} from "@/app/lib/blog/read";
import { renderJsonText } from "@/app/[channel]/[locale]/products/[id]/parts";

export async function generateMetadata(
  props: PageProps<"/[channel]/[locale]/blog/author/[slug]">,
): Promise<Metadata> {
  const { slug } = await props.params;
  const author = await getBlogAuthor(slug);
  if (!author) return { title: "Author not found" };
  const name = author.content?.name ?? author.displayName;
  const page = parseBlogPage(await props.searchParams);
  const href = blogAuthorHref(author);
  const bio = renderJsonText(author.content?.bio);
  return {
    title: page > 1 ? `${name} - page ${page}` : name,
    description: bio ?? `Posts by ${name}.`,
    alternates: await localizedAlternates(page > 1 ? `${href}?page=${page}` : href),
  };
}

export default async function BlogAuthorPage(
  props: PageProps<"/[channel]/[locale]/blog/author/[slug]">,
) {
  const { slug } = await props.params;
  const page = parseBlogPage(await props.searchParams);

  const author = await getBlogAuthor(slug);
  if (!author) {
    const to = await blogTaxonomyRedirect("blog/author/" + slug);
    if (to) permanentRedirect(to);
    notFound();
  }

  const { posts, hasMore } = await listBlogPostsBy(
    { authorSlug: author.slug },
    {
      limit: BLOG_ARCHIVE_PAGE_SIZE,
      offset: (page - 1) * BLOG_ARCHIVE_PAGE_SIZE,
    },
  );
  // Page 2+ of an author who never wrote that much is a dead URL, not an empty state
  if (page > 1 && posts.length === 0) notFound();

  return (
    <BlogArchive
      eyebrow="Author"
      title={author.content?.name ?? author.displayName}
      description={renderJsonText(author.content?.bio)}
      href={blogAuthorHref(author)}
      posts={posts}
      page={page}
      hasMore={hasMore}
    />
  );
}
