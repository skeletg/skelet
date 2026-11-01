import { describe, expect, it, vi } from "vitest";

import { configureStorefront, perRequest, request, rethrowControlFlow } from "./request";
import { memoryJar, testRequest } from "./testing";

const counting = (seen: Map<unknown, unknown>) =>
  <A extends unknown[], R>(f: (...args: A) => R) =>
  (...args: A): R => {
    const key = JSON.stringify(args);
    if (!seen.has(key)) seen.set(key, f(...args));
    return seen.get(key) as R;
  };

describe("request", () => {
  it("finds the adapter from a copy of this module nobody configured - a module graph rebuilt under a dev server", async () => {
    const req = testRequest({ locale: async () => "de" });
    configureStorefront({ request: () => req });
    vi.resetModules();
    const copy = await import("./request?copy" as string);
    expect(copy).not.toBe(await import("./request"));
    expect(await (copy.request() as typeof req).locale()).toBe("de");
    expect(request()).toBe(req);
  });

  it("keeps a configured copy on its own adapter when another copy configures one after it", async () => {
    const mine = testRequest({ locale: async () => "en" });
    configureStorefront({ request: () => mine });
    vi.resetModules();
    const other = await import("./request?other" as string);
    other.configureStorefront({ request: () => testRequest({ locale: async () => "fr" }) });
    expect(request()).toBe(mine);
    expect(await (other.request() as typeof mine).locale()).toBe("fr");
  });

  it("memoizes through the adapter's cache, and runs straight through without one", () => {
    const fn = vi.fn((n: number) => n * 2);
    configureStorefront({ request: () => testRequest() });
    const plain = perRequest(fn);
    plain(1);
    plain(1);
    expect(fn).toHaveBeenCalledTimes(2);

    configureStorefront({ request: () => testRequest(), cache: counting(new Map()) });
    const cached = perRequest(fn);
    cached(2);
    cached(2);
    expect(fn).toHaveBeenCalledTimes(3);
  });

  it("looks the memo up per call - a read defined before the adapter, or before it changed, uses the current one", () => {
    const fn = vi.fn((n: number) => n + 1);
    configureStorefront({ request: () => testRequest() });
    const read = perRequest(fn);
    read(1);
    configureStorefront({ request: () => testRequest(), cache: counting(new Map()) });
    read(1);
    read(1);
    expect(fn).toHaveBeenCalledTimes(2);
    configureStorefront({ request: () => testRequest(), cache: counting(new Map()) });
    read(1);
    expect(fn).toHaveBeenCalledTimes(3);
  });

  it("lets only the framework's own exits through a core catch", () => {
    class Exit extends Error {}
    configureStorefront({
      request: () => testRequest(),
      rethrowControlFlow: (err) => {
        if (err instanceof Exit) throw err;
      },
    });
    expect(() => rethrowControlFlow(new Exit())).toThrow(Exit);
    expect(() => rethrowControlFlow(new Error("app"))).not.toThrow();
    configureStorefront({ request: () => testRequest() });
    expect(() => rethrowControlFlow(new Exit())).not.toThrow();
  });
});

describe("memoryJar", () => {
  it("keeps each cookie's options for a test to read back", () => {
    const jar = memoryJar({ a: "1" });
    jar.set("b", "2", { httpOnly: true, maxAge: 60 });
    expect(jar.get("a")?.value).toBe("1");
    expect(jar.options("b")).toEqual({ httpOnly: true, maxAge: 60 });
    jar.delete("b");
    expect(jar.get("b")).toBeUndefined();
    expect(jar.options("b")).toBeUndefined();
  });
});
