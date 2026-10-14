import { describe, expect, it } from "vitest";

import { rethrowIfDynamicBailout } from "@/app/lib/dynamic-bailout";

describe("rethrowIfDynamicBailout", () => {
  it("rethrows Next's dynamic-usage signal so the route is not silently kept static", () => {
    const err = Object.assign(new Error("Dynamic server usage"), { digest: "DYNAMIC_SERVER_USAGE" });
    expect(() => rethrowIfDynamicBailout(err)).toThrow(err);
  });

  it("swallows everything else", () => {
    expect(() => rethrowIfDynamicBailout(new Error("backend down"))).not.toThrow();
    expect(() => rethrowIfDynamicBailout(Object.assign(new Error(), { digest: "NEXT_NOT_FOUND" }))).not.toThrow();
    expect(() => rethrowIfDynamicBailout(Object.assign(new Error(), { digest: 42 }))).not.toThrow();
    expect(() => rethrowIfDynamicBailout(null)).not.toThrow();
    expect(() => rethrowIfDynamicBailout(undefined)).not.toThrow();
  });
});
