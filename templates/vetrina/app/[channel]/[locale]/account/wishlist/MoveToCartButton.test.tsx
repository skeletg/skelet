import { fireEvent, screen, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";

import { renderWithIntl } from "@/app/test/render";

const KEY = "storefront_wishlist";
const local = () => JSON.parse(localStorage.getItem(KEY) ?? "[]") as string[];

// What the guest list held at the moment the page refreshed - the refresh is what remounts WishlistMerge
let atRefresh: string[] | null = null;
const refresh = vi.fn(() => { atRefresh = local(); });
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh }) }));
vi.mock("@/app/components/CartUIProvider", () => ({ useCartUI: () => ({ open: vi.fn() }) }));
vi.mock("@/app/lib/cart/store", () => ({ useCartStore: () => ({ applyServerCart: vi.fn() }) }));
const moveToCartAction = vi.fn();
const toggleWishlistAction = vi.fn(async () => ({ authed: true }));
vi.mock("@/app/lib/wishlist/actions", () => ({ moveToCartAction, toggleWishlistAction }));

const { MoveToCartButton } = await import("./MoveToCartButton");
const { WishlistMerge } = await import("./WishlistMerge");

beforeEach(() => {
  vi.clearAllMocks();
  atRefresh = null;
  localStorage.setItem(KEY, JSON.stringify(["p1", "p2"]));
});

it("drops the moved item from the guest list before refreshing, so the import can't bring it back", async () => {
  moveToCartAction.mockResolvedValue({ ok: true, cart: null });
  renderWithIntl(<MoveToCartButton productId="p1" />);
  fireEvent.click(screen.getByRole("button"));
  await waitFor(() => expect(refresh).toHaveBeenCalled());
  expect(atRefresh).toEqual(["p2"]);

  // The refreshed wishlist page no longer lists p1, and its merge finds nothing to import
  renderWithIntl(<WishlistMerge serverIds={["p2"]} />);
  expect(toggleWishlistAction).not.toHaveBeenCalled();
});

it("keeps the guest entry when the move fails", async () => {
  moveToCartAction.mockResolvedValue({ ok: false, error: "moveFailed" });
  renderWithIntl(<MoveToCartButton productId="p1" />);
  fireEvent.click(screen.getByRole("button"));
  expect(await screen.findByRole("alert")).toBeInTheDocument();
  expect(local()).toEqual(["p1", "p2"]);
  expect(refresh).not.toHaveBeenCalled();
});
