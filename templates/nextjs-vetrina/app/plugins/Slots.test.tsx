import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { __resetForTests, registerSlot } from "./registry";
import { Slots } from "./Slots";

describe("storefront plugin slots", () => {
  beforeEach(() => __resetForTests());
  afterEach(() => vi.restoreAllMocks());

  it("draws what's registered for the zone, with the zone's context, and nothing for a zone nobody claimed", () => {
    registerSlot<{ totalCents: number }>({ id: "acme:badge", zone: "checkout:summary", render: (ctx) => <p>saved on {ctx.totalCents}</p> });
    const { container } = render(
      <>
        <Slots zone="checkout:summary" ctx={{ totalCents: 2600 }} />
        <Slots zone="cart:summary" />
      </>,
    );
    expect(screen.getByText("saved on 2600")).toBeTruthy();
    expect(container.querySelectorAll("span.contents")).toHaveLength(1);
  });

  // A slot in the checkout summary, or in the header of every page, must not be able to take the page with it
  it("leaves out a slot that throws and keeps the rest of the page", () => {
    vi.spyOn(console, "error").mockImplementation(() => {});
    registerSlot({
      id: "acme:broken",
      zone: "checkout:payment",
      render: () => {
        throw new Error("currency.code is undefined");
      },
    });
    const Thrower = () => {
      throw new Error("a component inside the slot threw");
    };
    registerSlot({ id: "acme:nested", zone: "checkout:payment", render: () => <Thrower /> });
    registerSlot({ id: "acme:fine", zone: "checkout:payment", render: () => <p>pay in three</p> });
    render(
      <>
        <button>Place order</button>
        <Slots zone="checkout:payment" />
      </>,
    );
    expect(screen.getByRole("button", { name: "Place order" })).toBeTruthy();
    expect(screen.getByText("pay in three")).toBeTruthy();
  });
});
