// What the storefront logic needs from the request it runs in. A framework adapter answers it once, with
// configureStorefront, and everything in this package asks request() - so the same reads and actions run under Next,
// Astro or anything else that can hand over a request's cookies, headers, locale and channel

export type CookieOptions = {
  maxAge?: number;
  expires?: Date;
  path?: string;
  domain?: string;
  httpOnly?: boolean;
  secure?: boolean;
  sameSite?: "lax" | "strict" | "none";
};

export interface CookieJar {
  get(name: string): { value: string } | undefined;
  set(name: string, value: string, options?: CookieOptions): void;
  delete(name: string): void;
}

export interface HeaderReader {
  get(name: string): string | null;
}

// A read the framework may keep in its own data cache, under these tags, for this many seconds
export type CacheHint = { tags?: string[]; revalidate?: number };

export interface StorefrontRequest {
  cookies(): Promise<CookieJar>;
  headers(): Promise<HeaderReader>;
  locale(): Promise<string>;
  channelId(): Promise<string>;
  // Uncached without a hint, every call reaching the backend
  fetch(url: string, init: RequestInit, cache?: CacheHint): Promise<Response>;
  redirect(to: string, mode?: "push" | "replace"): never;
  notFound(): never;
  // Drop what the framework rendered and cached for a path - nothing to do where nothing is cached
  revalidate(path: string, scope?: "page" | "layout"): void;
  // Re-render the current page for the shopper after an action changed what it shows
  refresh(): void;
}

type Memo = <A extends unknown[], R>(fn: (...args: A) => R) => (...args: A) => R;

export type StorefrontAdapter = {
  request: () => StorefrontRequest;
  // The framework's per-request memo, where it has one - perRequest falls back to none
  cache?: Memo;
};

let adapter: StorefrontAdapter | null = null;

export function configureStorefront(a: StorefrontAdapter): void {
  adapter = a;
}

export function request(): StorefrontRequest {
  if (!adapter) {
    throw new Error(
      "No storefront adapter is configured - reach the storefront's reads and actions through an adapter package " +
        "(@skelet/storefront-next), which configures one",
    );
  }
  return adapter.request();
}

// The request's exits as plain functions - TypeScript only sees a call end a code path when it names the function
export function redirect(to: string, mode?: "push" | "replace"): never {
  return mode ? request().redirect(to, mode) : request().redirect(to);
}

export function notFound(): never {
  return request().notFound();
}

// perRequest is fn memoized for one request, so a page and its layout asking the same thing fetch it once
export function perRequest<A extends unknown[], R>(fn: (...args: A) => R): (...args: A) => R {
  let memo: ((...args: A) => R) | undefined;
  return (...args) => {
    memo ??= adapter?.cache ? adapter.cache(fn) : fn;
    return memo(...args);
  };
}
