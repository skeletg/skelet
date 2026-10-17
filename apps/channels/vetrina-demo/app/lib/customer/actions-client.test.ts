import { beforeEach, expect, it, vi } from "vitest";
import { loadHeaderIdentity } from "./actions-client";

const load = vi.hoisted(() => vi.fn());
vi.mock("./read", () => ({ loadCurrentCustomer: load }));
beforeEach(() => load.mockReset());

it("returns null only for an anonymous session", async () => {
  load.mockResolvedValue({ kind: "anonymous" });
  expect(await loadHeaderIdentity()).toBeNull();
});

it("reports a profile failure instead of treating it as signed out", async () => {
  const error = new Error("backend unavailable");
  load.mockResolvedValue({ kind: "error", error });
  await expect(loadHeaderIdentity()).rejects.toThrow(error);
});
