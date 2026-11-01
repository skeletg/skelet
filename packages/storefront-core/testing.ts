// A request for tests, standing in for a framework adapter: an empty cookie jar and headers, the default locale and a
// test channel, the global fetch, and a redirect or notFound that throws. A test overrides only what it looks at, with
// fakes as partial as it likes - a jar that only reads, say

import { configureStorefront, type CookieJar, type StorefrontRequest } from "./request";

export class TestRedirect extends Error {
  constructor(
    readonly to: string,
    readonly mode?: "push" | "replace",
  ) {
    super(`redirect to ${to}`);
  }
}

export class TestNotFound extends Error {
  constructor() {
    super("not found");
  }
}

export function memoryJar(initial: Record<string, string> = {}): CookieJar {
  const values = new Map(Object.entries(initial));
  return {
    get: (name) => (values.has(name) ? { value: values.get(name)! } : undefined),
    set: (name, value) => void values.set(name, value),
    delete: (name) => void values.delete(name),
  };
}

type Overrides = { [K in keyof StorefrontRequest]?: unknown };

export function testRequest(overrides: Overrides = {}): StorefrontRequest {
  const jar = memoryJar();
  return {
    cookies: async () => jar,
    headers: async () => new Headers(),
    locale: async () => "en",
    channelId: async () => "test-channel",
    fetch: (url, init) => fetch(url, init),
    redirect: (to, mode) => {
      throw new TestRedirect(to, mode);
    },
    notFound: () => {
      throw new TestNotFound();
    },
    revalidate: () => {},
    refresh: () => {},
    ...(overrides as Partial<StorefrontRequest>),
  };
}

// useTestRequest configures the storefront with a test request, read afresh on every call so a test can swap fields
export function useTestRequest(overrides: Overrides = {}): StorefrontRequest {
  const req = testRequest(overrides);
  configureStorefront({ request: () => req });
  return req;
}
