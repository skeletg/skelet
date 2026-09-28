import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Field, FormErrors } from "@/app/components/account/AccountUI";
import { renderWithIntl } from "@/app/test/render";

const meta = (over: Partial<Parameters<typeof Field>[0]["field"] & object> = {}) => ({
  id: "f-email",
  name: "email",
  key: "k1",
  initialValue: "ana@example.com",
  errors: undefined as string[] | undefined,
  errorId: "f-email-error",
  descriptionId: "f-email-description",
  ...over,
});

describe("Field", () => {
  it("names the input by its label alone and links the hint as a description", () => {
    renderWithIntl(<Field label="Password" name="password" type="password" hint="At least 8 characters." />);
    const input = screen.getByLabelText("Password");
    expect(input).toHaveAccessibleName("Password");
    expect(input).toHaveAccessibleDescription("At least 8 characters.");
  });

  it("takes value and name from a Conform field", () => {
    renderWithIntl(<Field label="Email" field={meta()} type="email" />);
    const input = screen.getByLabelText("Email");
    expect(input).toHaveValue("ana@example.com");
    expect(input).toHaveAttribute("name", "email");
    expect(input).not.toHaveAttribute("aria-invalid");
  });

  it("translates an error key, marks the input invalid and describes it with the error", () => {
    renderWithIntl(<Field label="Email" field={meta({ errors: ["email"] })} type="email" />);
    const input = screen.getByLabelText("Email");
    expect(input).toHaveAttribute("aria-invalid", "true");
    expect(input).toHaveAccessibleDescription("Enter a valid email address.");
  });
});

describe("FormErrors", () => {
  it("shows a key translated and backend text verbatim", () => {
    const { rerender } = renderWithIntl(<FormErrors errors={["signInFailed"]} />);
    expect(screen.getByRole("alert")).toHaveTextContent("Sign in failed.");
    rerender(<FormErrors errors={["Invalid email or password."]} />);
    expect(screen.getByRole("alert")).toHaveTextContent("Invalid email or password.");
  });

  it("renders nothing without errors", () => {
    const { container } = renderWithIntl(<FormErrors errors={[]} />);
    expect(container).toBeEmptyDOMElement();
  });
});
