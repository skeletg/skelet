import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { Field } from "@/app/components/account/AccountUI";
import { renderWithIntl } from "@/app/test/render";

describe("password Field", () => {
  it("reveals and hides what was typed", async () => {
    const user = userEvent.setup();
    renderWithIntl(<Field label="Password" name="password" type="password" />);
    const input = screen.getByLabelText("Password");
    const toggle = screen.getByRole("button", { name: "Show password" });

    await user.type(input, "hunter22");
    expect(input).toHaveAttribute("type", "password");

    await user.click(toggle);
    expect(input).toHaveAttribute("type", "text");
    expect(input).toHaveValue("hunter22");
    expect(toggle).toHaveAttribute("aria-pressed", "true");

    await user.click(toggle);
    expect(input).toHaveAttribute("type", "password");
  });
});
