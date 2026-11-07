import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

vi.mock("@/app/components/LocaleLink", () => ({
  Link: ({ href, children, className }: { href: string; children: ReactNode; className?: string }) => (
    <a href={href} className={className}>
      {children}
    </a>
  ),
}));

const { AssistantText, storePath } = await import("./AssistantText");

const STORE = ["https://shop.example"];

describe("storePath", () => {
  it("keeps the store's own addresses as paths and drops everyone else's", () => {
    expect(storePath("/shop/rain-shell", STORE)).toBe("/shop/rain-shell");
    expect(storePath("https://shop.example/shop/rain-shell?size=m", STORE)).toBe("/shop/rain-shell?size=m");
    expect(storePath("https://evil.example/login", STORE)).toBeNull();
    expect(storePath("//evil.example/login", STORE)).toBeNull();
    expect(storePath("javascript:alert(1)", STORE)).toBeNull();
  });

  it("reads an address the way a browser would, so another host never passes as a path", () => {
    expect(storePath("/\\evil.example/login", STORE)).toBeNull();
    expect(storePath("https://shop.example//evil.example/login", STORE)).toBeNull();
    expect(storePath("/shop//double", STORE)).toBe("/shop//double");
    expect(storePath("/info/returns#refunds", STORE)).toBe("/info/returns#refunds");
    expect(storePath("/shop/rain-shell", ["null"])).toBeNull();
    expect(storePath("/shop/rain-shell", ["null", "https://shop.example"])).toBe("/shop/rain-shell");
  });
});

describe("AssistantText", () => {
  it("renders paragraphs, lists, bold and the store's links - and nothing else as a link", () => {
    const { container } = render(
      <AssistantText
        storeOrigins={STORE}
        text={"Here are two:\n\n- **Rain Shell** - [see it](https://shop.example/shop/rain-shell)\n- Cheaper at [this site](https://evil.example/deal)\n\nOr <img src=x onerror=alert(1)>"}
      />,
    );
    expect(screen.getByText("Rain Shell").tagName).toBe("STRONG");
    expect(screen.getByText("see it").getAttribute("href")).toBe("/shop/rain-shell");
    expect(screen.queryByRole("link", { name: "this site" })).toBeNull();
    expect(container.textContent).toContain("Cheaper at this site");
    expect(container.querySelectorAll("li")).toHaveLength(2);
    expect(container.querySelector("img")).toBeNull();
    expect(container.textContent).toContain("<img src=x onerror=alert(1)>");
  });

  it("leaves a sentence's full stop off a bare address", () => {
    const { container } = render(<AssistantText storeOrigins={STORE} text={"See https://shop.example/info/returns."} />);
    expect(container.querySelector("a")?.getAttribute("href")).toBe("/info/returns");
    expect(container.textContent).toBe("See https://shop.example/info/returns.");
  });

  it("gives every part of a many-block reply its own key", () => {
    const error = vi.spyOn(console, "error").mockImplementation(() => {});
    render(<AssistantText storeOrigins={STORE} text={"One\n\n- a\n- b\n\nTwo\n\n1. c"} />);
    expect(error.mock.calls.filter((c) => String(c[0]).includes("same key"))).toEqual([]);
    error.mockRestore();
  });

  it("draws a list that follows its lead-in line, and numbers a numbered one", () => {
    const { container } = render(
      <AssistantText storeOrigins={STORE} text={"For example:\n- A tyre type?\n- A size?\nThat helps.\n\n1. Pick a size\n2. Add it"} />,
    );
    expect(container.querySelectorAll("ul > li")).toHaveLength(2);
    expect(container.querySelectorAll("ol > li")).toHaveLength(2);
    expect(container.textContent).not.toContain("- A");
    expect(screen.getByText("For example:").tagName).toBe("SPAN");
    expect(screen.getByText("That helps.").closest("p")).not.toBeNull();
  });
});
