import { describe, expect, it } from "vitest";

import { parseForm } from "../forms/schema";
import { reviewSchema } from "./schemas";

describe("reviewSchema", () => {
  it("reads per-question answers keyed by question id", () => {
    const fd = new FormData();
    fd.set("productId", "p1");
    fd.set("answer.3f2a-q1.inputType", "RATING_1_5");
    fd.set("answer.3f2a-q1.rating", "4");
    fd.set("answer.q2.inputType", "YES_NO");
    fd.set("answer.q2.bool", "false");
    const s = parseForm(fd, reviewSchema({ guest: false }));
    expect(s.status === "success" && s.value.answer).toEqual({
      "3f2a-q1": { inputType: "RATING_1_5", rating: "4" },
      q2: { inputType: "YES_NO", bool: "false" },
    });
  });

  it("requires a display name only from a guest", () => {
    const fd = new FormData();
    fd.set("productId", "p1");
    const guest = parseForm(fd, reviewSchema({ guest: true }));
    expect(guest.status === "error" && guest.error?.guestName).toEqual([
      "required",
    ]);
    expect(parseForm(fd, reviewSchema({ guest: false })).status).toBe("success");
  });
});
