// Admin-side draft preview for blog posts

import { notFound } from "next/navigation";

import { PreviewBlogPostDocument } from "@/app/lib/blog/operations.gql";
import { gqlFetch } from "@/app/lib/graphql";

import { PreviewClient } from "./PreviewClient";

async function fetchPreview(token: string) {
  try {
    const res = await gqlFetch(PreviewBlogPostDocument, { token });
    return res.data.previewBlogPost;
  } catch {
    return null;
  }
}

export default async function BlogPreviewPage(
  props: PageProps<"/[locale]/blog/preview/[id]">,
) {
  const { id } = await props.params;
  const search = await props.searchParams;
  const token = typeof search.token === "string" ? search.token : "";
  if (!token) notFound();
  const post = await fetchPreview(token);
  return <PreviewClient initialPost={post} postId={id} />;
}
