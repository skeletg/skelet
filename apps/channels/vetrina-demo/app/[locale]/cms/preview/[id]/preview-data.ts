import { BACKEND_INTERNAL_URL } from "@/app/lib/backend-url";
// Token-gated draft fetch shared by the preview page and its render action - the token
// is the only credential, so both surfaces must validate through the same query

// The GraphQL document is inlined as a string rather than going through the generated TypedDocumentNode in lib/cms.gql.ts because codegen
// needs a running backend to regenerate types; this keeps the route buildable before codegen
// Mirrors the backend's FRONT_API_SLUG - empty keeps /api/front/graphql, so this is inert unless set
const FRONT_API_PATH = process.env.FRONT_API_SLUG
  ? `/api/front/${process.env.FRONT_API_SLUG}/graphql`
  : "/api/front/graphql";

const PREVIEW_QUERY = /* GraphQL */ `
  query PreviewCMSPage($token: String!) {
    previewCMSPage(token: $token) {
      id
      slug
      urlPath
      content {
        locale
        title
        body
        metaTitle
        metaDescription
      }
    }
  }
`;

const BACKEND_URL = BACKEND_INTERNAL_URL;

export type PreviewPage = {
  id: string;
  slug: string;
  urlPath: string;
  content: {
    locale: string;
    title: string;
    body: unknown;
    metaTitle: string | null;
    metaDescription: string | null;
  } | null;
};

export async function fetchPreview(token: string): Promise<PreviewPage | null> {
  try {
    const res = await fetch(`${BACKEND_URL}${FRONT_API_PATH}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ query: PREVIEW_QUERY, variables: { token } }),
      // Preview content changes constantly - don't let Next.js cache
      cache: "no-store",
    });
    if (!res.ok) return null;
    const json = (await res.json()) as {
      data?: { previewCMSPage: PreviewPage | null };
      errors?: unknown;
    };
    if (json.errors || !json.data) return null;
    return json.data.previewCMSPage;
  } catch {
    return null;
  }
}
