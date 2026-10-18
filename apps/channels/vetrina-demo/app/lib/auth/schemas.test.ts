import { describe, expect, it } from "vitest";

import {
  addressSchema,
  loginSchema,
  profileSchema,
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

  it("reads a national phone in the address's country, and an international one in any", () => {
    const base = {
      firstName: "Ana",
      lastName: "Ilić",
      street1: "Knez Mihailova 1",
      city: "Beograd",
      postalCode: "11000",
      countryCode: "rs",
    };
    const national = parseForm(form({ ...base, phone: "064 123 4567" }), addressSchema);
    expect(national.status).toBe("success");
    const foreign = parseForm(form({ ...base, phone: "+49 1512 3456789" }), addressSchema);
    expect(foreign.status).toBe("success");

    const elsewhere = parseForm(form({ ...base, countryCode: "US", phone: "064 123 4567" }), addressSchema);
    expect(elsewhere.status === "error" && elsewhere.error?.phone).toEqual(["phoneInvalid"]);
    const short = parseForm(form({ ...base, phone: "064 12" }), addressSchema);
    expect(short.status === "error" && short.error?.phone).toEqual(["phoneInvalid"]);
  });

  it("reads a profile phone in the store's default country", () => {
    const base = { firstName: "Ana", lastName: "Ilić" };
    const ok = parseForm(form({ ...base, phone: "064 123 4567", phoneCountry: "RS" }), profileSchema);
    expect(ok.status).toBe("success");
    const noCountry = parseForm(form({ ...base, phone: "064 123 4567" }), profileSchema);
    expect(noCountry.status === "error" && noCountry.error?.phone).toEqual(["phoneInvalid"]);
    const blank = parseForm(form({ ...base, phone: "" }), profileSchema);
    expect(blank.status === "success" && blank.value.phone).toBeNull();
  });
});
