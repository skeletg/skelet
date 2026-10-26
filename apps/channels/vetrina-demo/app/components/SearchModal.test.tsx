import { act, fireEvent, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { renderWithIntl } from "@/app/test/render";

vi.mock("@/app/components/LocaleLink", () => ({
  Link: ({ href, children, ...rest }: { href: string; children: React.ReactNode }) => <a href={href} {...rest}>{children}</a>,
  useRouter: () => ({ push: vi.fn() }),
}));

// Each call parks until the test resolves it, so responses can be finished in any order
type Pending = { q: string; resolve: (names: string[]) => void };
const inFlight: Pending[] = [];
vi.mock("@/app/lib/search/actions", () => ({
  searchProductsAction: (q: string) =>
    new Promise((done) => {
      inFlight.push({
        q,
        resolve: (names) => done({ items: names.map((n) => ({ id: n, url: `/p/${n}`, sku: `SKU-${n.length}`, name: n })), totalCount: names.length }),
      });
    }),
}));

const { SearchModal } = await import("./SearchModal");

beforeEach(() => {
  inFlight.length = 0;
  vi.useFakeTimers();
});
afterEach(() => vi.useRealTimers());

const open = () => fireEvent.click(screen.getByRole("button", { name: "Search products" }));
const type = async (value: string) => {
  fireEvent.change(screen.getByRole("searchbox", { name: "Search products" }), { target: { value } });
  await act(async () => { vi.advanceTimersByTime(200); });
};
const finish = async (q: string, names: string[]) => {
  await act(async () => { inFlight.find((p) => p.q === q)!.resolve(names); });
};

it("keeps the newest query's results when an older response lands last", async () => {
  renderWithIntl(<SearchModal />);
  open();
  await type("sh");
  await type("shirt");
  await finish("shirt", ["Linen shirt"]);
  await finish("sh", ["Shoes"]);
  expect(screen.getByText("Linen shirt")).toBeInTheDocument();
  expect(screen.queryByText("Shoes")).not.toBeInTheDocument();
});

it("drops a response that lands after the dialog closed", async () => {
  renderWithIntl(<SearchModal />);
  open();
  await type("shirt");
  fireEvent.keyDown(window, { key: "Escape" });
  await finish("shirt", ["Stale shirt"]);
  open();
  fireEvent.change(screen.getByRole("searchbox", { name: "Search products" }), { target: { value: "shirt" } });
  expect(screen.queryByText("Stale shirt")).not.toBeInTheDocument();
});
