import { afterEach, describe, expect, it, vi } from "vitest";
import { directionsHref, preferenceKey, readPreference, savePreference, type Store } from "./types";
const validId = "9f8b64d2-96f1-4fbd-8252-f42bd396f9c8";
const sample = { latitude: null, longitude: null, line1: "Main 1", line2: "", city: "Belgrade", region: "", postalCode: "", countryCode: "RS" } as Store;
afterEach(() => vi.unstubAllGlobals());
describe("store preference", () => {
  it("discards malformed IDs before they can break availability requests", () => {
    const storage = new Map([[preferenceKey("a"), "invalid"]]);
    vi.stubGlobal("localStorage", { getItem: (k: string) => storage.get(k), removeItem: (k: string) => storage.delete(k) });
    expect(readPreference("a")).toBeNull();
    expect(storage.size).toBe(0);
  });
  it("handles disabled storage without breaking the page", () => {
    vi.stubGlobal("localStorage", { getItem: () => { throw new Error("denied"); }, setItem: () => { throw new Error("denied"); } });
    vi.stubGlobal("window", { dispatchEvent: vi.fn() });
    expect(readPreference("a")).toBeNull();
    expect(() => savePreference("a", validId)).not.toThrow();
  });
});
describe("directions", () => {
  it("uses coordinates when present and encoded addresses otherwise", () => {
    expect(directionsHref({ ...sample, latitude: 44, longitude: 20 })).toContain("destination=44%2C20");
    expect(directionsHref(sample)).toContain("destination=Main%201%2C%20Belgrade%2C%20RS");
    expect(directionsHref({ ...sample, line1: "", city: "", countryCode: "" })).toBeNull();
  });
});
