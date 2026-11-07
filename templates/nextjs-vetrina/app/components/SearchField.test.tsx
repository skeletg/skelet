import { fireEvent, render, screen } from "@testing-library/react";
import { NextIntlClientProvider } from "next-intl";
import { beforeEach, expect, it, vi } from "vitest";

import messages from "@/messages/en.json";
import { renderWithIntl } from "@/app/test/render";

const push = vi.hoisted(() => vi.fn());
vi.mock("next/navigation", () => ({ useRouter: () => ({ push, replace: vi.fn(), prefetch: vi.fn() }), usePathname: () => "/" }));

const { DefaultLocaleProvider } = await import("./LocaleLink");
const { SearchField } = await import("./SearchField");

beforeEach(() => push.mockClear());

const box = () => screen.getByRole("searchbox", { name: "Search products" }) as HTMLInputElement;

it("clears what was typed back into a focused, empty field, and won't search for nothing", () => {
  renderWithIntl(<SearchField initialQuery="linen shirt" />);
  expect(box().value).toBe("linen shirt");

  fireEvent.click(screen.getByRole("button", { name: "Clear search" }));
  expect(box().value).toBe("");
  expect(document.activeElement).toBe(box());
  expect(screen.queryByRole("button", { name: "Clear search" })).toBeNull();

  const form = screen.getByRole("search");
  fireEvent.submit(form);
  expect(push).not.toHaveBeenCalled();

  fireEvent.change(box(), { target: { value: "  a dress for a wedding " } });
  expect(fireEvent.submit(form)).toBe(false);
  expect(push).toHaveBeenCalledWith("/search?q=a+dress+for+a+wedding", undefined);
});

it("searches in the shopper's language, and the form alone would too", () => {
  render(
    <NextIntlClientProvider locale="de" messages={messages}>
      <DefaultLocaleProvider value="en">
        <SearchField initialQuery="" />
      </DefaultLocaleProvider>
    </NextIntlClientProvider>,
  );
  expect(screen.getByRole("search")).toHaveAttribute("action", "/de/search");
  fireEvent.change(box(), { target: { value: "leinenhemd" } });
  fireEvent.submit(screen.getByRole("search"));
  expect(push).toHaveBeenCalledWith("/de/search?q=leinenhemd", undefined);
});

it("takes the new query when the page moves on to another search", () => {
  const { rerender } = renderWithIntl(<SearchField initialQuery="boots" />);
  fireEvent.change(box(), { target: { value: "boo" } });
  rerender(<SearchField initialQuery="linen shirt" />);
  expect(box().value).toBe("linen shirt");
});
