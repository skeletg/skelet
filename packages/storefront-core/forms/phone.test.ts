import { describe, expect, it } from "vitest";

import { callingCode, formatAsTyped, formatPhone, isValidPhone } from "./phone";

describe("phone helpers", () => {
  it("validates against the country only when the number has no +", () => {
    expect(isValidPhone("01512 3456789", "DE")).toBe(true);
    expect(isValidPhone("01512 3456789", "US")).toBe(false);
    expect(isValidPhone("+33612345678", "DE")).toBe(true);
    expect(isValidPhone("01512 3456789", "")).toBe(false);
    expect(isValidPhone("01512 3456789", "XX")).toBe(false);
  });

  it("formats as typed in the given country", () => {
    expect(formatAsTyped("2015550123", "US")).toBe("(201) 555-0123");
    expect(formatAsTyped("+4915123456789", "US")).toBe("+49 1512 3456789");
    expect(formatAsTyped("", "DE")).toBe("");
  });

  it("shows stored numbers internationally and leaves older free text alone", () => {
    expect(formatPhone("+4915123456789")).toBe("+49 1512 3456789");
    expect(formatPhone("call the shop")).toBe("call the shop");
  });

  it("names the country's calling code for the placeholder", () => {
    expect(callingCode("DE")).toBe("+49");
    expect(callingCode("")).toBeNull();
  });
});
