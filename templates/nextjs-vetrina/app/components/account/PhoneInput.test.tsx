import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { Field } from "@/app/components/account/AccountUI";
import { renderWithIntl } from "@/app/test/render";

describe("phone Field", () => {
  it("formats as it's typed, in the country it's given", async () => {
    const user = userEvent.setup();
    renderWithIntl(<Field label="Phone" name="phone" type="tel" phoneCountry="US" />);
    const input = screen.getByLabelText("Phone");
    await user.type(input, "2015550123");
    expect(input).toHaveValue("(201) 555-0123");
  });

  it("names the country's calling code while empty", () => {
    renderWithIntl(<Field label="Phone" name="phone" type="tel" phoneCountry="RS" />);
    expect(screen.getByLabelText("Phone")).toHaveAttribute("placeholder", "+381");
  });

  it("shows a stored number in its international form", () => {
    renderWithIntl(<Field label="Phone" name="phone" type="tel" phoneCountry="RS" defaultValue="+381641234567" />);
    expect(screen.getByLabelText("Phone")).toHaveValue("+381 64 1234567");
  });

  it("keeps a number from another country in its international form", () => {
    renderWithIntl(<Field label="Phone" name="phone" type="tel" phoneCountry="US" defaultValue="+381641234567" />);
    expect(screen.getByLabelText("Phone")).toHaveValue("+381 64 1234567");
  });

  // Reformatting a deletion would put back the bracket or space just removed
  it("leaves a deletion as the shopper made it", async () => {
    const user = userEvent.setup();
    renderWithIntl(<Field label="Phone" name="phone" type="tel" phoneCountry="US" />);
    const input = screen.getByLabelText("Phone");
    await user.type(input, "201");
    expect(input).toHaveValue("(201)");
    await user.type(input, "{Backspace}");
    expect(input).toHaveValue("(201");
  });

  // Reformatting mid-edit would throw the caret to the end
  it("leaves an edit in the middle alone", async () => {
    const user = userEvent.setup();
    renderWithIntl(<Field label="Phone" name="phone" type="tel" phoneCountry="US" defaultValue="201 5550123" />);
    const input = screen.getByLabelText("Phone") as HTMLInputElement;
    await user.type(input, "9", { initialSelectionStart: 3, initialSelectionEnd: 3 });
    expect(input.value).toBe("2019 5550123");
  });

  it("is submitted under its own name", () => {
    renderWithIntl(<Field label="Phone" name="phone" type="tel" phoneCountry="US" />);
    expect(screen.getByLabelText("Phone")).toHaveAttribute("name", "phone");
    expect(screen.getByLabelText("Phone")).toHaveAttribute("type", "tel");
  });
});
