// Server-side client for Skelet's front GraphQL API - never imported by browser code
import type { TypedDocumentNode } from "@graphql-typed-document-node/core";
import { getOperationAST, print } from "graphql";

const BACKEND = process.env.BACKEND_INTERNAL_URL ?? "http://localhost:8080";
// Mirrors the backend's FRONT_API_SLUG - empty keeps /api/front/graphql
const ENDPOINT = `${BACKEND}${process.env.FRONT_API_SLUG ? `/api/front/${process.env.FRONT_API_SLUG}/graphql` : "/api/front/graphql"}`;

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const CHANNEL_ID = (process.env.STOREFRONT_CHANNEL_ID ?? "").trim().toLowerCase();

export class GqlError extends Error {
  constructor(
    message: string,
    // The backend's extensions.code (PRODUCT_NOT_FOUND ...), or UNAVAILABLE when the store itself failed
    readonly code: string,
  ) {
    super(message);
  }
}

// What a shopper may read - a resolver's own validation message, never transport or internal detail
export function shopperMessage(err: unknown, fallback: string): string {
  if (err instanceof GqlError && err.code !== "UNAVAILABLE" && err.code !== "INTERNAL") return err.message;
  console.error(err);
  return fallback;
}

const printed = new WeakMap<object, { query: string; operationName?: string }>();

export async function gql<TData, TVars>(doc: TypedDocumentNode<TData, TVars>, variables?: TVars): Promise<TData> {
  // One storefront serves one channel - guessing would quietly render another market (docs/channels.md)
  if (!UUID.test(CHANNEL_ID)) throw new GqlError("STOREFRONT_CHANNEL_ID is not a channel uuid", "UNAVAILABLE");
  let body = printed.get(doc);
  if (!body) {
    // operationName matters: the backend's response cache allowlists by it
    body = { query: print(doc), operationName: getOperationAST(doc)?.name?.value };
    printed.set(doc, body);
  }
  let res: Response;
  try {
    res = await fetch(ENDPOINT, {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-Channel-Id": CHANNEL_ID },
      body: JSON.stringify({ ...body, variables }),
      signal: AbortSignal.timeout(10_000),
    });
  } catch (err) {
    throw new GqlError(`backend unreachable (${body.operationName}): ${String(err)}`, "UNAVAILABLE");
  }
  if (!res.ok) throw new GqlError(`backend HTTP ${res.status} (${body.operationName})`, "UNAVAILABLE");
  const json = (await res.json()) as { data?: TData; errors?: { message: string; extensions?: { code?: string; class?: string } }[] };
  const first = json.errors?.[0];
  if (first) throw new GqlError(first.message, first.extensions?.code || first.extensions?.class || "INTERNAL");
  return json.data as TData;
}
