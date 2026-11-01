import { z } from "zod";

import { optionalText, requiredText } from "../forms/schema";

// One entry per returnable order line; a line left at 0 isn't being returned
export const returnRequestSchema = z
  .object({
    orderId: requiredText(),
    lines: z.array(
      z.object({
        orderLineId: requiredText(),
        // Order lines are whole units - half a returned item is a rejected RMA at best. Blank reads as 0
        quantity: z
          .number({ error: "wholeNumber" })
          .int("wholeNumber")
          .min(0, "wholeNumber")
          .default(0),
      }),
    ),
    resolution: z.enum(["refund", "exchange"]).default("refund"),
    reason: optionalText(),
  })
  .refine((v) => v.lines.some((l) => l.quantity > 0), {
    error: "returnNothingSelected",
  });
