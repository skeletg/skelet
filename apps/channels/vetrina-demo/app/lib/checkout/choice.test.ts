import { describe, expect, it } from "vitest";

import { choiceKey, choiceLabel } from "./choice";

const c = (optionId: string, optionName: string, carrierSlug: string, methodName: string) => ({
  optionId, optionName, carrierSlug, carrierName: carrierSlug === "builtin" ? "Store rates" : carrierSlug.toUpperCase(), methodName,
});

describe("choiceLabel", () => {
  it("is just the option's name when it has one method", () => {
    const only = c("door", "Delivery", "builtin", "Standard");
    expect(choiceLabel(only, [only])).toEqual(["Delivery"]);
  });

  it("names the method when the option has several from one carrier", () => {
    const group = [c("door", "Delivery", "builtin", "Standard"), c("door", "Delivery", "builtin", "Express")];
    expect(choiceLabel(group[1], group)).toEqual(["Delivery", "Express"]);
  });

  it("names the carrier too when the option's methods come from several", () => {
    const group = [c("door", "Delivery", "builtin", "Standard"), c("door", "Delivery", "dhl", "Standard")];
    expect(choiceLabel(group[0], group)).toEqual(["Delivery", "Store rates", "Standard"]);
    expect(choiceLabel(group[1], group)).toEqual(["Delivery", "DHL", "Standard"]);
  });

  it("leaves the carrier out when each option has its own", () => {
    const group = [c("store", "Store courier", "builtin", "Standard"), c("store", "Store courier", "builtin", "Express"), c("dhl", "DHL", "dhl", "Standard")];
    expect(choiceLabel(group[0], group)).toEqual(["Store courier", "Standard"]);
    expect(choiceLabel(group[2], group)).toEqual(["DHL"]);
  });
});

describe("choiceKey", () => {
  it("tells the same method under two options apart", () => {
    expect(choiceKey({ optionId: "a", carrierSlug: "builtin", methodCode: "express" })).not.toBe(
      choiceKey({ optionId: "b", carrierSlug: "builtin", methodCode: "express" }),
    );
  });
});
