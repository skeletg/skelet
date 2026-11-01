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
  revalidatePublic(path: string): Promise<void>;
  revalidateAccount(): void;
  // Re-render the current page for the shopper after an action changed what it shows
  refresh(): void;
}

type Memo = <A extends unknown[], R>(fn: (...args: A) => R) => (...args: A) => R;

export type StorefrontAdapter = {
  request: () => StorefrontRequest;
  // The framework's per-request memo, where it has one - perRequest falls back to none
  cache?: Memo;
  rethrowControlFlow?: (err: unknown) => void;
};

// A bundler or dev server can load this file more than once - each copy keeps its own adapter, and one nobody configured finds the last one configured anywhere
const ADAPTER = Symbol.for("@skelet/storefront-core/adapter");
const registry = globalThis as { [ADAPTER]?: StorefrontAdapter };
let own: StorefrontAdapter | undefined;

export function configureStorefront(a: StorefrontAdapter): void {
  own = a;
  registry[ADAPTER] = a;
}

function current(): StorefrontAdapter | undefined {
  return own ?? registry[ADAPTER];
}

export function rethrowControlFlow(err: unknown): void {
  current()?.rethrowControlFlow?.(err);
}

export function request(): StorefrontRequest {
  const adapter = current();
  if (!adapter) {
    throw new Error(
      "No storefront adapter is configured - reach the storefront's reads and actions through an adapter package " +
        "(@skelet/storefront-next, @skelet/storefront-astro), which configures one",
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
  const wrapped = new WeakMap<Memo, (...args: A) => R>();
  return (...args) => {
    const memo = current()?.cache;
    if (!memo) return fn(...args);
    let w = wrapped.get(memo);
    if (!w) wrapped.set(memo, (w = memo(fn)));
    return w(...args);
  };
}
