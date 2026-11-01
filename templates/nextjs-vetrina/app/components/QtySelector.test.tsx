import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { QtySelector } from "@/app/components/QtySelector";
import { renderWithIntl } from "@/app/test/render";

describe("QtySelector", () => {
  it("steps within its bounds and disables the buttons at each end", async () => {
    const user = userEvent.setup();
    renderWithIntl(<QtySelector name="qty" min={1} max={2} />);
    const input = screen.getByRole("spinbutton");
    const dec = screen.getByRole("button", { name: "Decrease quantity" });
    const inc = screen.getByRole("button", { name: "Increase quantity" });

    expect(input).toHaveValue(1);
    expect(dec).toBeDisabled();
    await user.click(inc);
    expect(input).toHaveValue(2);
    expect(inc).toBeDisabled();
  });

  it("clamps a typed value to the maximum", async () => {
    const user = userEvent.setup();
    renderWithIntl(<QtySelector name="qty" max={5} />);
    const input = screen.getByRole("spinbutton");
    await user.clear(input);
    await user.type(input, "9");
    expect(input).toHaveValue(5);
  });

  it("reports to the parent instead of keeping its own state when controlled", async () => {
    const user = userEvent.setup();
    const onValueChange = vi.fn();
    renderWithIntl(<QtySelector value={2} onValueChange={onValueChange} max={10} />);
    await user.click(screen.getByRole("button", { name: "Increase quantity" }));
    expect(onValueChange).toHaveBeenCalledWith(3);
    expect(screen.getByRole("spinbutton")).toHaveValue(2);
  });
});
