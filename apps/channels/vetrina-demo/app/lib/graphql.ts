// Server-side GraphQL client for the storefront

import type { TypedDocumentNode } from "@graphql-typed-document-node/core";
import type { DocumentNode } from "graphql";
import { print } from "graphql";
import { cookies, headers as nextHeaders } from "next/headers";

import { getAccessToken } from "@/app/lib/auth/session";
import { getCurrentLocale, getCurrentChannelId } from "@/app/lib/locale/read";
import { isPublicOp } from "@/app/lib/public-ops";
import { BACKEND_INTERNAL_URL } from "@/app/lib/backend-url";

// Mirrors the backend's FRONT_API_SLUG - empty keeps /api/front/graphql, so this is inert unless set
const FRONT_API_PATH = process.env.FRONT_API_SLUG
  ? `/api/front/${process.env.FRONT_API_SLUG}/graphql`
  : '/api/front/graphql';

const BACKEND_URL = BACKEND_INTERNAL_URL;

// The operation name drives the public-op check below, so an unnamed document is
// treated as private rather than silently sharing a cache entry
function extractOpName<TData, TVariables>(
  document: TypedDocumentNode<TData, TVariables>,
): string {
  for (const def of (document as DocumentNode).definitions ?? []) {
    if (def.kind === "OperationDefinition" && def.name?.value) return def.name.value;
  }
  return "(anonymous)";
}

export type GraphQLError = {
  message: string;
  path?: (string | number)[];
  extensions?: Record<string, unknown>;
};

export class GraphQLRequestError extends Error {
  constructor(
    message: string,
    readonly errors: GraphQLError[],
  ) {
    super(message);
    this.name = "GraphQLRequestError";
  }
}

// What a shopper is allowed to see when the backend itself is the problem - the status,
// the internal host and the op name are operator detail and stay in the server log
const BACKEND_ERROR_MESSAGE =
  "We couldn't reach the store just now. Please try again in a moment.";

export class BackendUnavailableError extends Error {
  constructor(readonly detail: string) {
    super(BACKEND_ERROR_MESSAGE);
    this.name = "BackendUnavailableError";
  }
}

// The backend's error presenter stamps extensions.class (UNAUTHENTICATED / NOT_FOUND / VALIDATION / INTERNAL ...) on
// every front error - the only reliable way to tell "sign in again" from "the backend fell over"
export function errorClass(err: unknown): string | null {
  if (!(err instanceof GraphQLRequestError)) return null;
  for (const e of err.errors) {
    const cls = e.extensions?.class;
    if (typeof cls === "string") return cls;
  }
  return null;
}

export function isAuthError(err: unknown): boolean {
  const cls = errorClass(err);
  return cls === "UNAUTHENTICATED" || cls === "FORBIDDEN";
}

// True when the store is the problem rather than the request - transport, a non-200, or
// a resolver reporting UNAVAILABLE. Callers use it to tell "no such thing" (404) from
// "try again later" (error boundary)
// Not INTERNAL: the front presenter scrubs those, and today's backend reports plain
// "order not found" that way, so treating INTERNAL as an outage would 500 on a typo
export function isBackendFailure(err: unknown): boolean {
  if (err instanceof BackendUnavailableError) return true;
  return errorClass(err) === "UNAVAILABLE";
}

// For pages that render a load failure inline - the operator gets the whole error in the
// server log, the shopper gets a sentence that names no host, status or query
export function loadErrorMessage(
  context: string,
  err: unknown,
  fallback: string,
): string {
   
  console.error(`${context}:`, err);
  return fallback;
}

export type GqlResult<T> = {
  data: T;
};

// Refresh happens in middleware (see middleware.ts), NOT here

type FetchOptions = {
  extraHeaders?: Record<string, string>;
  // Overrides the cookie-derived `Locale` header for this request
  locale?: string;
  cache?: "force-cache";
  tags?: string[];
  revalidate?: number;
  // Channel + locale (and anything else) folded into the cache key
  cacheKeyExtra?: string[];
};

// Next says "this route can't be static" by throwing from cookies(), headers() and
// no-store fetches. Swallow that and the route stays static, then 500s on every request
function rethrowIfDynamicBailout(err: unknown): void {
  const digest = (err as { digest?: unknown } | null | undefined)?.digest;
  if (typeof digest === "string" && digest.startsWith("DYNAMIC_SERVER_USAGE")) throw err;
}

// The `variables` argument is required when the document declares any - the generated TypedDocumentNode<TData,
// TVariables> binds both sides so passing the wrong shape (or omitting required vars) fails tsc
export async function gqlFetch<TData, TVariables>(
  document: TypedDocumentNode<TData, TVariables>,
  ...args: TVariables extends Record<string, never>
    ? [variables?: TVariables, options?: FetchOptions]
    : [variables: TVariables, options?: FetchOptions]
): Promise<GqlResult<TData>> {
  const variables = args[0];
  const options = (args[1] as FetchOptions | undefined) ?? {};
  const opName = extractOpName(document); // resolved first so a public op never touches the auth cookie - reading one forces the route dynamic
  const [locale, channelId] = await Promise.all([getCurrentLocale(), getCurrentChannelId()]);
  const accessToken = isPublicOp(opName) ? null : await getAccessToken();
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    Locale: options.locale ?? locale,
  };
  // Omitted when no id is pinned - the backend then resolves via Host or its default
  if (channelId) headers["X-Channel-Id"] = channelId;
  if (accessToken) {
    headers.Authorization = `Bearer ${accessToken}`;
  }
  // Forward the real client IP so the backend's per-IP rate limiter keys on the customer, not on this Next server
  // (otherwise every storefront request shares one global bucket - brute-force pooling + cross-user lockout)
  try {
    const inbound = await nextHeaders();
    const clientIP = inbound.get("x-forwarded-for") ?? inbound.get("x-real-ip");
    if (clientIP) headers["X-Forwarded-For"] = clientIP;
    // The SEO crawler's bypass token, so a crawl of a few thousand pages is not refused by the per-IP limiter
    const crawl = inbound.get("x-skelet-crawl");
    if (crawl) headers["X-Skelet-Crawl"] = crawl;
  } catch (err) {
    rethrowIfDynamicBailout(err);
    // not in a request scope - nothing to forward
  }
  // Forward the visitor's anon id (the Lens lens_aid cookie) so guest commerce (cart / checkout without login)
  // attributes to the same anonymous visitor as their browsing, and stitches to a customer once they sign in
  try {
    const jar = await cookies();
    const aid = jar.get("lens_aid")?.value;
    if (aid) headers["X-Anon-Id"] = aid;
  } catch (err) {
    rethrowIfDynamicBailout(err);
    // not in a request scope - nothing to forward
  }
  if (options.extraHeaders) {
    for (const [k, v] of Object.entries(options.extraHeaders)) {
      if (v) headers[k] = v;
    }
  }
  // Default: never cache (every request hits the backend fresh)
  const cacheInit: RequestInit =
    options.cache === "force-cache"
      ? { cache: "force-cache", next: { tags: options.tags, revalidate: options.revalidate } }
      : { cache: "no-store" };

  let res: Response;
  try {
    res = await fetch(`${BACKEND_URL}${FRONT_API_PATH}`, {
      method: "POST",
      headers,
      body: JSON.stringify({
        query: print(document as DocumentNode),
        variables,
        ...(options.cacheKeyExtra ? { __cacheKey: options.cacheKeyExtra } : {}),
      }),
      ...cacheInit,
    });
  } catch (err) {
    rethrowIfDynamicBailout(err);
    const detail = `GraphQL transport failure calling ${BACKEND_URL} (${opName || "op"}): ${err instanceof Error ? err.message : String(err)}`;
     
    console.error(detail);
    throw new BackendUnavailableError(detail);
  }

  if (!res.ok) {
    // Body deliberately not read into the message - pages render err.message to the shopper
    const detail = `GraphQL HTTP ${res.status} ${res.statusText} from ${BACKEND_URL} (${opName || "op"})`;
     
    console.error(detail);
    throw new BackendUnavailableError(detail);
  }

  const body = (await res.json()) as {
    data?: TData;
    errors?: GraphQLError[];
  };
  if (body.errors?.length) {
    // Auth failures (UNAUTHENTICATED) are not retried here - the middleware keeps the
    // access token fresh before render
    throw new GraphQLRequestError(
      body.errors.map((e) => e.message).join("; "),
      body.errors,
    );
  }
  if (body.data === undefined) {
    throw new Error("GraphQL response had neither data nor errors");
  }
  return { data: body.data };
}

