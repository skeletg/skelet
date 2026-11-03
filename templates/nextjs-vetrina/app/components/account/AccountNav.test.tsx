import { screen, within } from "@testing-library/react";
import { it, expect, vi } from "vitest";
import { renderWithIntl } from "@/app/test/render";
import { AccountNav } from "./AccountNav";

vi.mock("@/app/components/LocaleLink", () => ({
  Link: ({ href, children, ...props }: React.ComponentProps<"a">) => <a href={href} {...props}>{children}</a>,
  usePathname: () => "/account",
}));

it.each(["sidebar", "strip"] as const)("shows totals and a separate new indicator in the %s", (variant) => {
  renderWithIntl(<AccountNav variant={variant} counts={{ wishlist: 3, offers: 2, newOffers: 1 }} />);
  const wishlist = screen.getByRole("link", { name: /Wishlist/ });
  const offers = screen.getByRole("link", { name: /Offers/ });
  expect(within(wishlist).getByLabelText("3 wishlist items")).toHaveTextContent("3");
  expect(within(offers).getByLabelText("2 offers, 1 new offer")).toHaveTextContent("2");
});

it("keeps the offer total after new offers have been opened", () => {
  renderWithIntl(<AccountNav counts={{ wishlist: 0, offers: 2, newOffers: 0 }} />);
  expect(screen.getByLabelText("2 offers")).toHaveTextContent("2");
  expect(screen.queryByLabelText(/new offer/)).not.toBeInTheDocument();
  expect(screen.getByRole("link", { name: "Wishlist" })).toBeInTheDocument();
});
