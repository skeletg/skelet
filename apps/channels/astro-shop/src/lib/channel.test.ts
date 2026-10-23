import { beforeEach, describe, expect, it, vi } from "vitest";

const gql = vi.fn();
vi.mock("./gql", () => ({ gql }));

const { forgetChannel, getChannel } = await import("./channel");

const channel = { name: "Shop", defaultLocale: "en", publicUrl: null, currency: { code: "USD" } };

beforeEach(() => {
  gql.mockReset();
  forgetChannel();
});

describe("getChannel", () => {
  it("asks the backend once and holds the answer", async () => {
    gql.mockResolvedValue({ currentChannel: channel });
    await getChannel();
    await getChannel();
    expect(gql).toHaveBeenCalledTimes(1);
  });

  it("asks again after forgetChannel (a config:public purge)", async () => {
    gql.mockResolvedValue({ currentChannel: channel });
    await getChannel();
    forgetChannel();
    await getChannel();
    expect(gql).toHaveBeenCalledTimes(2);
  });

  it("refuses to serve when the pinned channel doesn't resolve", async () => {
    gql.mockResolvedValue({ currentChannel: null });
    await expect(getChannel()).rejects.toThrow(/names no active channel/);
  });
});
