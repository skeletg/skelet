"use server";

// Server-render one in-flight (unsaved) block to its real form so the live preview shows
// the ACTUAL component - grids, accordions, anything the client bundle can't draw - with
// real catalog/blog data instead of a placeholder, called per non-static block as the
// operator edits (debounced)
//
// selectable stays off - the client already wraps each block in the data-block-id click
// target, so CmsBlocks must not add a second one

import type { ReactNode } from "react";

import { CmsBlocks } from "@/app/components/cms/CmsBlocks";
import type { Block } from "@/app/components/cms/blocks";
import { fetchPreview } from "./preview-data";

export type PreviewRenderResult =
  | { ok: true; node: ReactNode }
  | { ok: false; error: string };

export async function renderPreviewBlock(
  pageId: string,
  token: string,
  block: Block,
): Promise<PreviewRenderResult> {
  // Server actions are anonymous HTTP endpoints - gate on the same preview token the
  // page requires, or anyone could render arbitrary blocks with live data
  const page = token ? await fetchPreview(token) : null;
  // one generic message on purpose - must not reveal whether the page id exists
  if (!page || page.id !== pageId) {
    return { ok: false, error: "Preview session is invalid or has expired" };
  }
  return { ok: true, node: <CmsBlocks blocks={[block]} /> };
}
