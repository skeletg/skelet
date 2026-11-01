// Server-side GraphQL client for the storefront

import { request, rethrowControlFlow } from "./request";
import type { TypedDocumentNode } from "@graphql-typed-document-node/core";
import type { DocumentNode } from "graphql";
import { print } from "graphql";

import { getAccessToken } from "./auth/session";
import { getCurrentLocale, getCurrentChannelId } from "./locale/read";
import { shopperHeaders } from "./shopper-headers";
import { isPublicOp } from "./public-ops";
import { FRONT_API_URL } from "./backend-url";
import { decodeOfferSession, OFFER_COOKIE, OFFER_HEADER } from "./cart/offer-cookie";

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

export type FetchOptions = {
  public?: boolean;
  extraHeaders?: Record<string, string>;
  // Overrides the cookie-derived `Locale` header for this request
  locale?: string;
  cache?: "force-cache";
  tags?: string[];
  revalidate?: number;
  // Channel + locale (and anything else) folded into the cache key
  cacheKeyExtra?: string[];
  surface?: string;
};

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
  const shared = options.public === true || isPublicOp(opName);
  const accessToken = shared ? null : await getAccessToken();
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    Locale: options.locale ?? locale,
  };
  // A request no channel claims still names one, so the backend answers "no store" rather than for its default channel
  headers["X-Channel-Id"] = channelId || "unresolved";
  if (options.surface) headers["X-Skelet-Cache-Surface"] = options.surface;
  if (accessToken) {
    headers.Authorization = `Bearer ${accessToken}`;
  }
  Object.assign(headers, await shopperHeaders());
  // An offer cart answers to its link, not its id - the link's token goes with every private call
  if (!shared) {
    try {
      const offerToken = decodeOfferSession((await request().cookies()).get(OFFER_COOKIE)?.value)?.token;
      if (offerToken) headers[OFFER_HEADER] = offerToken;
    } catch (err) {
      rethrowControlFlow(err);
      // not in a request scope - nothing to forward
    }
  }
  if (options.extraHeaders) {
    for (const [k, v] of Object.entries(options.extraHeaders)) {
      if (v) headers[k] = v;
    }
  }
  // Default: never cache (every request hits the backend fresh)
  const hint = options.cache === "force-cache" ? { tags: options.tags, revalidate: options.revalidate } : undefined;

  let res: Response;
  try {
    res = await request().fetch(
      FRONT_API_URL,
      {
        method: "POST",
        headers,
        body: JSON.stringify({
          query: print(document as DocumentNode),
          variables,
          // The backend's response cache allowlists by operationName, so without it no storefront read was ever cached there
          ...(opName !== "(anonymous)" ? { operationName: opName } : {}),
          ...(options.cacheKeyExtra ? { __cacheKey: options.cacheKeyExtra } : {}),
        }),
      },
      hint,
    );
  } catch (err) {
    rethrowControlFlow(err);
    const detail = `GraphQL transport failure calling ${FRONT_API_URL} (${opName || "op"}): ${err instanceof Error ? err.message : String(err)}`;
    console.error(detail);
    throw new BackendUnavailableError(detail);
  }

  if (!res.ok) {
    // Body deliberately not read into the message - pages render err.message to the shopper
    const detail = `GraphQL HTTP ${res.status} ${res.statusText} from ${FRONT_API_URL} (${opName || "op"})`;
    console.error(detail);
    throw new BackendUnavailableError(detail);
  }

  const body = (await res.json()) as {
    data?: TData;
    errors?: GraphQLError[];
  };
  if (body.errors?.length) {
    // Auth failures (UNAUTHENTICATED) are not retried here - the adapter's proxy keeps the
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

