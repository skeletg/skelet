// Admin-side draft preview for CMS pages

import { notFound } from "next/navigation";

import { CmsBlocks } from "@/app/components/cms/CmsBlocks";
import { parseDocument } from "@/app/components/cms/blocks";
import { CmsPreviewClient } from "./CmsPreviewClient";
import { fetchPreview } from "./preview-data";

export default async function CmsPreviewPage(
  props: PageProps<"/[channel]/[locale]/cms/preview/[id]">,
) {
  const { id } = await props.params;
  const search = await props.searchParams;
  const token = typeof search.token === "string" ? search.token : "";
  if (!token) notFound();

  const page = await fetchPreview(token);
  const doc = parseDocument(page?.content?.body);
  const initialServerRender = doc ? <CmsBlocks blocks={doc.blocks} selectable /> : null;

  return (
    <CmsPreviewClient
      pageId={id}
      token={token}
      initialBlocks={doc?.blocks ?? null}
      initialServerRender={initialServerRender}
    />
  );
}
