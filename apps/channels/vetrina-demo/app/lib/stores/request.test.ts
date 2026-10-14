import { describe, expect, it } from "vitest";
import { storeRequestSchema } from "./request";
describe("store request boundary", () => {
  it("accepts database UUIDs, including imported identifiers without version bits", () => {
    expect(storeRequestSchema.safeParse({ productId: "c011583f-109c-ac28-fdeb-b85c1bd47cf0" }).success).toBe(true);
  });
  it("rejects unsupported actions, malformed identifiers and unbounded filters", () => {
    for (const input of [{ action: "graphql" }, { productId: "bad" }, { search: "x".repeat(201) }, { offset: "-1" }, { offset: "100001" }, { latitude: "91", longitude: "20" }, { latitude: "44" }, { longitude: "20" }]) {
      expect(storeRequestSchema.safeParse(input).success).toBe(false);
    }
  });
  it("parses paired coordinates and bounded pagination", () => {
    expect(storeRequestSchema.parse({ latitude: "44.8", longitude: "20.4", offset: "20" })).toMatchObject({ latitude: 44.8, longitude: 20.4, offset: 20, action: "stores" });
  });
});
