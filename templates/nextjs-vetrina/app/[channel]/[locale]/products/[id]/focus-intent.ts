// Distinguishes "the shopper engaged with this control" from "we moved focus there for
// them"

let programmatic = false;

export function withProgrammaticFocus(fn: () => void) {
  programmatic = true;
  try {
    fn();
  } finally {
    setTimeout(() => {
      programmatic = false;
    }, 0);
  }
}

export function isProgrammaticFocus(): boolean {
  return programmatic;
}

export function optionAnchorId(optionId: string): string {
  return `opt-anchor-${optionId}`;
}

// Scrolls a flagged option into view and parks focus on its first real control - shared by the
// hint's jump links and by the blocked add-to-cart, so both land the shopper in the same place
// A step picker shows one option at a time - it listens for this, brings the option up and jumps again
export const OPTION_JUMP_EVENT = "pdp:jump-to-option";

export function jumpToOption(optionId: string) {
  const el = document.getElementById(optionAnchorId(optionId));
  if (!el) {
    window.dispatchEvent(new CustomEvent<string>(OPTION_JUMP_EVENT, { detail: optionId }));
    return;
  }
  el.scrollIntoView({ behavior: "smooth", block: "center" });
  // Radio/checkbox choices render their real input as .sr-only behind a styled label, so those
  // count as focusable here - landing on one lets a keyboard user arrow straight through the group
  const focusable =
    el.querySelector<HTMLElement>(
      "input:not([type=hidden]), textarea, select, button, [tabindex]:not([tabindex='-1'])",
    ) ?? el;
  // preventScroll: the smooth scroll above already owns the movement - focus() would otherwise
  // snap it instantly to a different position
  withProgrammaticFocus(() => focusable.focus({ preventScroll: true }));
}
