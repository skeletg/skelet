import { z } from "zod";

export const couponSchema = z.object({
  code: z.string({ error: "couponRequired" }).trim().min(1, "couponRequired"),
});
