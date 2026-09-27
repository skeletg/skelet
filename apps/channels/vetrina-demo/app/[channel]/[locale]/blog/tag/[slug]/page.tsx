// Tag archive - every published post carrying one tag

import type { Metadata } from "next";
import { localizedAlternates } from "@/app/lib/locale/alternates";
import { notFound, permanentRedirect } from "next/navigation";
import { BlogArchive } from "@/app/components/BlogArchive";
import {
  BLOG_ARCHIVE_PAGE_SIZE,
  blogTagHref,
  blogTaxonomyRedirect,
  getBlogTag,
  listBlogPostsBy,
  parseBlogPage,
} from "@/app/lib/blog/read";
import { displayName } from "@/app/lib/display";
import { renderJsonText } from "@/app/lib/json-text";

export async function generateMetadata(
  props: PageProps<"/[channel]/[locale]/blog/tag/[slug]">,
): Promise<Metadata> {
  const { slug } = await props.params;
  const tag = await getBlogTag(slug);
  if (!tag) return { title: "Tag not found" };
  const name = displayName(tag.content?.name, tag.slug);
  const page = parseBlogPage(await props.searchParams);
  const href = blogTagHref(tag);
  return {
    title:
      tag.content?.metaTitle ??
      (page > 1 ? `#${name} - page ${page}` : `#${name}`),
    description:
      tag.content?.metaDescription ??
      renderJsonText(tag.content?.description) ??
      `Posts tagged ${name}.`,
    alternates: await localizedAlternates(page > 1 ? `${href}?page=${page}` : href),
  };
}

export default async function BlogTagPage(
  props: PageProps<"/[channel]/[locale]/blog/tag/[slug]">,
) {
  const { slug } = await props.params;
  const page = parseBlogPage(await props.searchParams);

  const tag = await getBlogTag(slug);
  if (!tag) {
    const to = await blogTaxonomyRedirect("blog/tag/" + slug);
    if (to) permanentRedirect(to);
    notFound();
  }

  const { posts, hasMore } = await listBlogPostsBy(
    { tagSlug: tag.slug },
    {
      limit: BLOG_ARCHIVE_PAGE_SIZE,
      offset: (page - 1) * BLOG_ARCHIVE_PAGE_SIZE,
    },
  );
  if (page > 1 && posts.length === 0) notFound();

  return (
    <BlogArchive
      eyebrow="Tag"
      title={`#${displayName(tag.content?.name, tag.slug)}`}
      description={renderJsonText(tag.content?.description)}
      href={blogTagHref(tag)}
      posts={posts}
      page={page}
      hasMore={hasMore}
    />
  );
}
