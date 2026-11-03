// @vitest-environment jsdom
import { beforeAll, beforeEach, describe, expect, it } from "vitest";

import { currentTheme, initTheme } from "./theme";

let systemDark = false;
const win = { matchMedia: () => ({ matches: systemDark }), localStorage: window.localStorage } as unknown as Window;
const root = document.documentElement;

beforeAll(() => {
  document.body.innerHTML = `
    <button type="button" data-theme-toggle aria-pressed="false">Dark theme</button>
    <select data-font-picker><option value="default">Poppins</option><option value="montserrat">Montserrat</option></select>`;
  initTheme(document, win);
});

beforeEach(() => {
  delete root.dataset.theme;
  delete root.dataset.font;
  window.localStorage.clear();
  systemDark = false;
});

const toggle = () => document.querySelector<HTMLButtonElement>("[data-theme-toggle]")!;

describe("theme toggle", () => {
  it("follows the system until the shopper picks", () => {
    systemDark = true;
    expect(currentTheme(document, win)).toBe("dark");
    root.dataset.theme = "light";
    expect(currentTheme(document, win)).toBe("light");
  });

  it("flips from the system theme and remembers the pick", () => {
    systemDark = true;
    toggle().click();
    expect(root.dataset.theme).toBe("light");
    expect(window.localStorage.getItem("theme")).toBe("light");
    expect(toggle().getAttribute("aria-pressed")).toBe("false");

    toggle().click();
    expect(root.dataset.theme).toBe("dark");
    expect(toggle().getAttribute("aria-pressed")).toBe("true");
  });
});

describe("font picker", () => {
  const pick = (value: string) => {
    const s = document.querySelector<HTMLSelectElement>("[data-font-picker]")!;
    s.value = value;
    s.dispatchEvent(new Event("change", { bubbles: true }));
  };

  it("sets the font and forgets it again on default", () => {
    pick("montserrat");
    expect(root.dataset.font).toBe("montserrat");
    expect(window.localStorage.getItem("font")).toBe("montserrat");

    pick("default");
    expect(root.dataset.font).toBeUndefined();
    expect(window.localStorage.getItem("font")).toBeNull();
  });
});
