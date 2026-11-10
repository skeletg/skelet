import { describe, expect, it, vi } from "vitest";

const read = vi.hoisted(() => ({ searchOverview: vi.fn() }));
vi.mock("@skelet/storefront-next/search/read", () => read);
vi.mock("./OverviewBox", () => ({ OverviewBox: () => null, OverviewPending: () => null, OverviewSlot: () => null }));

const { Overview } = await import("./SearchOverview");
const { OverviewBox } = await import("./OverviewBox");

describe("Overview", () => {
  it("shows nothing when no overview came back, so the slot folds away", async () => {
    read.searchOverview.mockResolvedValue(null);
    expect(await Overview({ query: "beach wedding" })).toBeNull();
  });

  it("hands the box the overview and the chat it would carry on, without fetching any cards up front", async () => {
    const overview = { text: "Try [[1]].", recap: "Shown as cards: Linen shirt (id p1)", products: [{ id: "p1", name: "Linen shirt", url: "shop/linen-shirt" }] };
    read.searchOverview.mockResolvedValue(overview);
    const el = await Overview({ query: " beach wedding " });
    expect(el?.type).toBe(OverviewBox);
    expect(el?.props).toEqual({
      overview,
      chat: { question: "beach wedding", reply: "Try Linen shirt.", recap: "Shown as cards: Linen shirt (id p1)", productIds: ["p1"] },
    });
  });
});
