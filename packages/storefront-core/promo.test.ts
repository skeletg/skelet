import { expect, it } from "vitest";

import { PROMO_PENDING_COOKIE, pendingPromo } from "./promo";

it("holds a code the way the backend matches it, in an httpOnly cookie", () => {
  const held = pendingPromo("  summer-20 ");
  expect(held?.slice(0, 2)).toEqual([PROMO_PENDING_COOKIE, "SUMMER-20"]);
  expect(held?.[2]).toMatchObject({ path: "/", httpOnly: true, sameSite: "lax", maxAge: 60 * 60 * 24 * 7 });
});

it.each([["nothing", ""], ["blank", "   "], ["whitespace inside", "TWO WORDS"], ["a control char", "A\u0000B"], ["too long", "X".repeat(65)], ["null", null]])(
  "holds nothing for %s",
  (_label, raw) => {
    expect(pendingPromo(raw)).toBeNull();
  },
);
