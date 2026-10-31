import { beforeEach, describe, expect, it } from "vitest";

import {
  getAccountNavItems,
  getSlots,
  registerAccountNavItem,
  registerSlot,
  __resetForTests,
} from "./registry";

const Icon = () => null;

describe("storefront plugin registry", () => {
  beforeEach(() => __resetForTests());

  it("hands back nothing for a zone no plugin claimed", () => {
    expect(getSlots("cart:summary")).toEqual([]);
    expect(getAccountNavItems()).toEqual([]);
  });

  it("orders slots by order, lowest first", () => {
    registerSlot({ id: "b", zone: "cart:actions", order: 20, render: () => null });
    registerSlot({ id: "a", zone: "cart:actions", order: 10, render: () => null });
    expect(getSlots("cart:actions").map((s) => s.id)).toEqual(["a", "b"]);
  });

  it("defaults order to 100, which puts a plugin after core's 10-50 band", () => {
    registerSlot({ id: "core", zone: "z", order: 50, render: () => null });
    registerSlot({ id: "plugin", zone: "z", render: () => null });
    expect(getSlots("z").map((s) => s.id)).toEqual(["core", "plugin"]);
  });

  it("keeps zones apart", () => {
    registerSlot({ id: "x", zone: "cart:summary", render: () => null });
    expect(getSlots("cart:actions")).toEqual([]);
  });

  // Two plugins claiming one id is the collision that would otherwise surface as a duplicate React
  // key or a silently dropped slot - it fails at registration instead, naming the id
  it("refuses a duplicate id in the same zone", () => {
    registerSlot({ id: "dup", zone: "cart:summary", render: () => null });
    expect(() =>
      registerSlot({ id: "dup", zone: "cart:summary", render: () => null }),
    ).toThrow(/duplicate slot "dup"/);
  });

  it("allows the same id in different zones", () => {
    registerSlot({ id: "same", zone: "one", render: () => null });
    expect(() => registerSlot({ id: "same", zone: "two", render: () => null })).not.toThrow();
  });

  it("passes the zone ctx through to render", () => {
    registerSlot<{ subtotal: number }>({
      id: "ctx",
      zone: "cart:summary",
      render: (ctx) => String(ctx.subtotal),
    });
    expect(getSlots<{ subtotal: number }>("cart:summary")[0].render({ subtotal: 42 })).toBe("42");
  });

  it("sorts account nav items and refuses duplicates", () => {
    registerAccountNavItem({ id: "b", href: "/b", label: "B", Icon, order: 60 });
    registerAccountNavItem({ id: "a", href: "/a", label: "A", Icon, order: 10 });
    expect(getAccountNavItems().map((i) => i.id)).toEqual(["a", "b"]);
    expect(() =>
      registerAccountNavItem({ id: "a", href: "/a2", label: "A2", Icon }),
    ).toThrow(/duplicate account nav item "a"/);
  });
});
