import { describe, expect, it, vi } from "vitest";

import { configureStorefront, perRequest, request } from "./request";
import { testRequest } from "./testing";

describe("request", () => {
  it("finds the adapter from any copy of this module - a bundle layer or a rebuilt module graph loads its own", async () => {
    const req = testRequest({ locale: async () => "de" });
    configureStorefront({ request: () => req });
    vi.resetModules();
    const copy = await import("./request?copy" as string);
    expect(copy).not.toBe(await import("./request"));
    expect(await (copy.request() as typeof req).locale()).toBe("de");
    expect(request()).toBe(req);
  });

  it("memoizes through the adapter's cache, and runs straight through without one", () => {
    const fn = vi.fn((n: number) => n * 2);
    configureStorefront({ request: () => testRequest() });
    const plain = perRequest(fn);
    plain(1);
    plain(1);
    expect(fn).toHaveBeenCalledTimes(2);

    const memo = new Map<unknown, unknown>();
    configureStorefront({
      request: () => testRequest(),
      cache: <A extends unknown[], R>(f: (...args: A) => R) => (...args: A): R => {
        const key = JSON.stringify(args);
        if (!memo.has(key)) memo.set(key, f(...args));
        return memo.get(key) as R;
      },
    });
    const cached = perRequest(fn);
    cached(2);
    cached(2);
    expect(fn).toHaveBeenCalledTimes(3);
  });
});
