// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { NotFoundView } from "./NotFoundView";
import { OrderPlacedView } from "./OrderPlacedView";

afterEach(cleanup);

describe("NotFoundView", () => {
  it("says so and links back to the shop", () => {
    render(<NotFoundView />);
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("We couldn't find that page");
    expect(screen.getByRole("link", { name: "Back to the shop" }).getAttribute("href")).toBe("/");
  });
});

describe("OrderPlacedView", () => {
  it("echoes the order number where the e2e suite reads it", () => {
    const { container } = render(<OrderPlacedView number="SO-1001" />);
    expect(container.querySelector("[data-order-number]")?.textContent).toBe("SO-1001");
  });

  it("leaves the number out when there is none", () => {
    const { container } = render(<OrderPlacedView />);
    expect(container.querySelector("[data-order-number]")).toBeNull();
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("Thank you for your order");
  });
});
