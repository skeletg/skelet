import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

let pathname = "/";
vi.mock("next/navigation", () => ({ usePathname: () => pathname }));
// Next runs its bundled canary React; the stable build the tests resolve has no ViewTransition
vi.mock("react", async (original) => ({
  ...(await original<typeof import("react")>()),
  ViewTransition: ({ children }: { children: React.ReactNode }) => children,
}));

const { PageTransition } = await import("@/app/components/PageTransition");

const slot = () => screen.getByText("page").parentElement!;

describe("PageTransition", () => {
  it("blocks the page slot from recorders in the same render that swaps a private page in", () => {
    pathname = "/products/shirt";
    const { rerender } = render(<PageTransition><p>page</p></PageTransition>);
    expect(slot().classList.contains("lens-block")).toBe(false);

    pathname = "/de/checkout";
    rerender(<PageTransition><p>page</p></PageTransition>);
    expect(slot().classList.contains("lens-block")).toBe(true);

    pathname = "/de";
    rerender(<PageTransition><p>page</p></PageTransition>);
    expect(slot().classList.contains("lens-block")).toBe(false);
  });
});
