import { describe, expect, it } from "vitest";
import { parseForm } from "../forms/schema";
import { productPickSchema, productSearchSchema, storeFiltersSchema } from "./schemas";

const form = (entries: Record<string, string>) => {
  const data = new FormData();
  for (const [k, v] of Object.entries(entries)) data.set(k, v);
  return data;
};
const storeId = "9f8b64d2-96f1-4fbd-8252-f42bd396f9c8";

describe("store filter schema", () => {
  it("reads checkboxes, coordinates and the pager offset the way the form submits them", () => {
    const submission = parseForm(form({ city: " Belgrade ", pickupOnly: "on", latitude: "44.8", longitude: "20.4", offset: "20", preferredStoreId: storeId }), storeFiltersSchema);
    expect(submission.status).toBe("success");
    expect(submission.status === "success" && submission.value).toEqual({ city: "Belgrade", pickupOnly: true, latitude: 44.8, longitude: 20.4, offset: 20, preferredStoreId: storeId });
  });
  it("accepts UUID-shaped ids without RFC version bits, as the demo fixtures use", () => {
    expect(parseForm(form({ productId: "00000000-0000-0000-0000-000000000000" }), storeFiltersSchema).status).toBe("success");
  });
  it("rejects half a position, out-of-range values and malformed ids", () => {
    const invalid: Record<string, string>[] = [{ latitude: "44.8" }, { latitude: "91", longitude: "0" }, { offset: "-1" }, { productId: "product-a" }, { search: "x".repeat(201) }];
    for (const entries of invalid) {
      expect(parseForm(form(entries), storeFiltersSchema).status, JSON.stringify(entries)).toBe("error");
    }
  });
});

describe("product schemas", () => {
  it("requires a search term and a product id", () => {
    expect(parseForm(form({ q: "  " }), productSearchSchema).status).toBe("error");
    expect(parseForm(form({ q: "hoodie" }), productSearchSchema).status).toBe("success");
    expect(parseForm(form({}), productPickSchema).status).toBe("error");
    expect(parseForm(form({ productId: storeId }), productPickSchema).status).toBe("success");
  });
});
