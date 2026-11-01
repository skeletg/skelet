// Server-side GraphQL through the storefront core's client - the channel, the locale, the shopper's address and visitor
// id and a signed-in customer's token go with every call. Never imported by browser code
import type { TypedDocumentNode } from "@graphql-typed-document-node/core";

import { BackendUnavailableError, gqlFetch, GraphQLRequestError } from "@skelet/storefront-core/graphql";

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

type Fetch = <TData>(doc: unknown, variables?: unknown) => Promise<{ data: TData }>;

export async function gql<TData, TVars>(doc: TypedDocumentNode<TData, TVars>, variables?: TVars): Promise<TData> {
  try {
    return (await (gqlFetch as Fetch)<TData>(doc, variables ?? {})).data;
  } catch (err) {
    if (err instanceof GraphQLRequestError) {
      const first = err.errors[0];
      const code = first?.extensions?.code ?? first?.extensions?.class;
      throw new GqlError(first?.message ?? err.message, typeof code === "string" && code ? code : "INTERNAL");
    }
    if (err instanceof BackendUnavailableError) throw new GqlError(err.detail, "UNAVAILABLE");
    throw err;
  }
}
