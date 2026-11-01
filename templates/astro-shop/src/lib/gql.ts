// Server-side GraphQL through the storefront core's client - the channel, the locale, the shopper's address and visitor
// id and a signed-in customer's token go with every call. Never imported by browser code
import type { TypedDocumentNode } from "@graphql-typed-document-node/core";

import { BackendUnavailableError, gqlFetch, GraphQLRequestError, type FetchOptions } from "@skelet/storefront-core/graphql";

export class GqlError extends Error {
  constructor(
    message: string,
    // The backend's extensions.code (PRODUCT_NOT_FOUND ...), or UNAVAILABLE when the store itself failed
    readonly code: string,
  ) {
    super(message);
  }
}

const COPY: Record<string, string> = {
  addFailed: "We couldn't add that to your cart - please try again.",
  missingProduct: "That product isn't available.",
  invalidBundle: "That bundle isn't available as chosen.",
  invalidOption: "Pick the options this product needs.",
  invalidSelection: "Pick what you'd like to add.",
  chooseAtLeastOne: "Choose at least one item.",
  uploadFailed: "That file didn't upload - please try again.",
};

export function copyFor(keyOrMessage: string): string {
  return COPY[keyOrMessage] ?? keyOrMessage;
}

// What a shopper may read - a resolver's own validation message, never transport or internal detail
export function shopperMessage(err: unknown, fallback: string): string {
  if (err instanceof GqlError && err.code !== "UNAVAILABLE" && err.code !== "INTERNAL") return err.message;
  console.error(err);
  return fallback;
}

type Args<TVars> = TVars extends Record<string, never> ? [variables?: TVars, options?: FetchOptions] : [variables: TVars, options?: FetchOptions];

// The same rule as gqlFetch's own tuple - TypeScript can't relate the two inside a generic
const typedFetch = gqlFetch as <D, V>(doc: TypedDocumentNode<D, V>, ...args: Args<V>) => Promise<{ data: D }>;

export const PUBLIC: FetchOptions = { public: true };

export async function gql<TData, TVars>(doc: TypedDocumentNode<TData, TVars>, ...args: Args<TVars>): Promise<TData> {
  try {
    return (await typedFetch(doc, ...args)).data;
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
