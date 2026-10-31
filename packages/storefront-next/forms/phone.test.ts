import { describe, expect, it } from "vitest";

import { callingCode, formatAsTyped, formatPhone, isValidPhone } from "./phone";

describe("phone helpers", () => {
  it("validates against the country only when the number has no +", () => {
    expect(isValidPhone("064 123 4567", "RS")).toBe(true);
    expect(isValidPhone("064 123 4567", "US")).toBe(false);
    expect(isValidPhone("+4915123456789", "RS")).toBe(true);
    expect(isValidPhone("064 123 4567", "")).toBe(false);
    expect(isValidPhone("064 123 4567", "XX")).toBe(false);
  });

  it("formats as typed in the given country", () => {
    expect(formatAsTyped("2015550123", "US")).toBe("(201) 555-0123");
    expect(formatAsTyped("+381641234567", "US")).toBe("+381 64 1234567");
    expect(formatAsTyped("", "RS")).toBe("");
  });

  it("shows stored numbers internationally and leaves older free text alone", () => {
    expect(formatPhone("+381641234567")).toBe("+381 64 1234567");
    expect(formatPhone("call the shop")).toBe("call the shop");
  });

  it("names the country's calling code for the placeholder", () => {
    expect(callingCode("RS")).toBe("+381");
    expect(callingCode("")).toBeNull();
  });
});
