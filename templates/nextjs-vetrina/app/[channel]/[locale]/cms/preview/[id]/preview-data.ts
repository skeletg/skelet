// Token-gated draft fetch shared by the preview page and its render action - the token
// is the only credential, so both surfaces must validate through the same query

import { PreviewCmsPageDocument, type PreviewCmsPageQuery } from "@skelet/storefront-core/cms/operations.gql";
import { gqlFetch } from "@skelet/storefront-next/graphql";

export type PreviewPage = NonNullable<PreviewCmsPageQuery["previewCMSPage"]>;

// gqlFetch is uncached unless asked, so a draft is always read fresh
export async function fetchPreview(token: string): Promise<PreviewPage | null> {
  try {
    const res = await gqlFetch(PreviewCmsPageDocument, { token });
    return res.data.previewCMSPage ?? null;
  } catch {
    return null;
  }
}
