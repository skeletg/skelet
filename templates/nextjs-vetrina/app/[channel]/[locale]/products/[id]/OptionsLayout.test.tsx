import { act, fireEvent, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderWithIntl } from "@/app/test/render";
import type { ProductCustomOption } from "@skelet/storefront-next/catalog/read";
import { OPTION_JUMP_EVENT } from "./focus-intent";
import { answerText, OptionsLayout } from "./OptionsLayout";

// What these pin: steps walk the visible options with the earlier answers above, a required one holds the customer
// until it's answered, Change goes back, and a jump from the add button brings the missing option up

const option = (id: string, title: string, over: Partial<ProductCustomOption> = {}): ProductCustomOption => ({
  id, inputType: "radio", isRequired: false, sortOrder: 0, title, helpText: "", maxChars: null, maxFileBytes: null,
  fileExtensions: null, priceValue: null, priceType: "fixed", visibleIf: null,
  values: [
    { id: `${id}-a`, title: "Gift", priceValue: 0, priceType: "fixed", quantity: 1, isDefault: false, forcedLineQty: null, sourceProduct: null },
  ],
  ...over,
});

const wrap = option("w", "Wrapping", { isRequired: true });
const note = option("n", "Card note", { inputType: "area", values: [] });
const date = option("d", "Delivery date", { inputType: "date", values: [] });

function render(answers = { sel: {} as Record<string, Set<string>>, text: {} as Record<string, string>, files: {} }, missing = ["w"]) {
  return renderWithIntl(
    <OptionsLayout
      layout="STEPS"
      options={[wrap, note, date]}
      answers={answers}
      missingRequiredIds={missing}
      renderOption={(o) => <div key={o.id} id={`opt-anchor-${o.id}`}>control for {o.title}</div>}
    />,
  );
}

describe("step picker", () => {
  it("holds the customer on a required step until it's answered", () => {
    render();
    expect(screen.getByText("Step 1 of 3")).toBeVisible();
    expect(screen.getByText("control for Wrapping")).toBeVisible();
    expect(screen.queryByText("control for Card note")).toBeNull();
    expect(screen.getByRole("button", { name: "Next" })).toBeDisabled();
    expect(screen.getByText("Answer this one to continue.")).toBeVisible();
  });

  it("moves on, lists the earlier answers, offers Skip for an optional one, and goes back with Change", () => {
    render({ sel: { w: new Set(["w-a"]) }, text: {}, files: {} }, []);
    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    expect(screen.getByText("Step 2 of 3")).toBeVisible();
    expect(screen.getByText("Gift")).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "Skip" }));
    expect(screen.getByText("Skipped")).toBeVisible();
    fireEvent.click(screen.getAllByRole("button", { name: "Change" })[0]);
    expect(screen.getByText("control for Wrapping")).toBeVisible();
  });

  it("ends on a review of every answer", () => {
    render({ sel: { w: new Set(["w-a"]) }, text: { n: "Happy birthday", d: "2026-11-01" }, files: {} }, []);
    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    fireEvent.click(screen.getByRole("button", { name: "Review your choices" }));
    expect(screen.getByText("All set - add it to your cart below.")).toBeVisible();
    expect(screen.getByText("Happy birthday")).toBeVisible();
    expect(screen.getByText("2026-11-01")).toBeVisible();
  });

  it("brings up the option a jump names", () => {
    render({ sel: { w: new Set(["w-a"]) }, text: {}, files: {} }, ["d"]);
    act(() => {
      window.dispatchEvent(new CustomEvent(OPTION_JUMP_EVENT, { detail: "d" }));
    });
    expect(screen.getByText("control for Delivery date")).toBeVisible();
  });

  it("words an answer per kind of option", () => {
    expect(answerText(wrap, { sel: { w: new Set(["w-a"]) }, text: {}, files: {} })).toBe("Gift");
    expect(answerText(note, { sel: {}, text: { n: "  " }, files: {} })).toBeNull();
    expect(answerText(option("f", "Artwork", { inputType: "file", values: [] }), { sel: {}, text: {}, files: { f: { status: "ready", assetId: "a", filename: "logo.png" } } })).toBe("logo.png");
  });

  it("stays a plain list when the product asks for one, or has a single option", () => {
    renderWithIntl(
      <OptionsLayout layout="LIST" options={[wrap, note]} answers={{ sel: {}, text: {}, files: {} }} missingRequiredIds={[]} renderOption={(o) => <p key={o.id}>control for {o.title}</p>} />,
    );
    expect(screen.getByText("control for Wrapping")).toBeVisible();
    expect(screen.getByText("control for Card note")).toBeVisible();
    expect(screen.queryByText(/Step 1/)).toBeNull();
  });
});
