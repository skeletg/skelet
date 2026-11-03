// The theme toggle and the dev font picker. Base.astro's head script applies both picks before first paint - this only
// changes them. A pick lives in the shopper's own browser, since every page is cached the same for everyone
//
//   data-theme-toggle   a button flipping light/dark - aria-pressed says whether dark is on
//   data-font-picker    a <select> of data-font values, "default" being the stylesheet's own font
type Theme = "light" | "dark";

function remember(win: Window, key: string, value: string | null) {
  try {
    if (value === null) win.localStorage.removeItem(key);
    else win.localStorage.setItem(key, value);
  } catch {
    // Storage blocked (private mode, site data off) - the pick lasts this page only
  }
}

// The shopper's pick, else the system's - the same rule the stylesheet's dark variant follows
export function currentTheme(doc: Document, win: Window): Theme {
  const picked = doc.documentElement.dataset.theme;
  if (picked === "light" || picked === "dark") return picked;
  return win.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

export function initTheme(doc: Document = document, win: Window = window): void {
  const root = doc.documentElement;
  const sync = () => {
    const dark = String(currentTheme(doc, win) === "dark");
    for (const b of doc.querySelectorAll("[data-theme-toggle]")) b.setAttribute("aria-pressed", dark);
    for (const s of doc.querySelectorAll<HTMLSelectElement>("[data-font-picker]")) s.value = root.dataset.font ?? "default";
  };
  sync();

  doc.addEventListener("click", (e) => {
    if (!(e.target instanceof Element) || !e.target.closest("[data-theme-toggle]")) return;
    const next: Theme = currentTheme(doc, win) === "dark" ? "light" : "dark";
    root.dataset.theme = next;
    remember(win, "theme", next);
    sync();
  });

  doc.addEventListener("change", (e) => {
    if (!(e.target instanceof HTMLSelectElement) || !("fontPicker" in e.target.dataset)) return;
    const font = e.target.value === "default" ? null : e.target.value;
    if (font) root.dataset.font = font;
    else delete root.dataset.font;
    remember(win, "font", font);
  });
}
