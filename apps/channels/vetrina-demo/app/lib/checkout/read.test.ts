import { beforeEach, expect, it, vi } from "vitest";
const { gqlFetch } = vi.hoisted(() => ({ gqlFetch: vi.fn() }));
vi.mock("@/app/lib/graphql", () => ({ gqlFetch }));
const { getDeliveryPoints } = await import("./read");
beforeEach(() => gqlFetch.mockReset());
it("passes search to the backend so points outside the initial result limit can be found", async () => {
  gqlFetch.mockResolvedValue({ data: { deliveryPoints: [{ id: "point-21" }] } });
  expect(await getDeliveryPoints("cart", "group", "Central station")).toEqual({ points: [{ id: "point-21" }], unavailable: false });
  expect(gqlFetch).toHaveBeenCalledWith(expect.anything(), { cartId: "cart", groupKey: "group", search: "Central station" });
});
it("distinguishes no matches from an unavailable source", async () => {
  gqlFetch.mockResolvedValueOnce({ data: { deliveryPoints: [] } }).mockRejectedValueOnce(new Error("offline"));
  expect(await getDeliveryPoints("cart", "group")).toEqual({ points: [], unavailable: false });
  expect(await getDeliveryPoints("cart", "group")).toEqual({ points: [], unavailable: true });
});

it("can recover a point-source failure on retry", async () => {
  gqlFetch.mockRejectedValueOnce(new Error("offline")).mockResolvedValueOnce({ data: { deliveryPoints: [{ id: "point-21" }] } });
  expect((await getDeliveryPoints("cart", "group", "station")).unavailable).toBe(true);
  expect(await getDeliveryPoints("cart", "group", "station")).toEqual({ points: [{ id: "point-21" }], unavailable: false });
});
