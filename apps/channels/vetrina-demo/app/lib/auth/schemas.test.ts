import { describe, expect, it } from "vitest";

import {
  addressSchema,
  loginSchema,
  registerSchema,
} from "@/app/lib/auth/schemas";
import { parseForm } from "@/app/lib/forms/schema";

function form(entries: Record<string, string>): FormData {
  const fd = new FormData();
  for (const [k, v] of Object.entries(entries)) fd.set(k, v);
  return fd;
}

describe("auth schemas", () => {
  it("reports blank fields as required and a malformed email as email", () => {
    const blank = parseForm(form({ email: "", password: "" }), loginSchema);
    expect(blank.status).toBe("error");
    expect(blank.status === "error" && blank.error).toEqual({
      email: ["required"],
      password: ["required"],
    });

    const bad = parseForm(form({ email: "nope", password: "x" }), loginSchema);
    expect(bad.status === "error" && bad.error?.email).toEqual(["email"]);
  });

  it("puts a password mismatch on the confirm field", () => {
    const s = parseForm(
      form({
        firstName: "Ana",
        lastName: "Ilić",
        email: "ana@example.com",
        password: "longenough",
        confirmPassword: "different1",
      }),
      registerSchema,
    );
    expect(s.status === "error" && s.error).toEqual({
      confirmPassword: ["passwordMismatch"],
    });
  });

  it("coerces the opt-in checkbox and blanks optional text to null", () => {
    const s = parseForm(
      form({
        firstName: " Ana ",
        lastName: "Ilić",
        email: "ana@example.com",
        password: "longenough",
        confirmPassword: "longenough",
        marketingOptIn: "on",
      }),
      registerSchema,
    );
    expect(s.status).toBe("success");
    expect(s.status === "success" && s.value).toMatchObject({
      firstName: "Ana",
      marketingOptIn: true,
      referralCode: null,
    });
  });

  it("upper-cases the country and rejects anything but two letters", () => {
    const base = {
      firstName: "Ana",
      lastName: "Ilić",
      street1: "Knez Mihailova 1",
      city: "Beograd",
      postalCode: "11000",
    };
    const ok = parseForm(form({ ...base, countryCode: "rs" }), addressSchema);
    expect(ok.status === "success" && ok.value).toMatchObject({
      countryCode: "RS",
      company: null,
      phone: null,
    });

    const bad = parseForm(form({ ...base, countryCode: "SRB" }), addressSchema);
    expect(bad.status === "error" && bad.error?.countryCode).toEqual([
      "countryCode",
    ]);
  });
});
