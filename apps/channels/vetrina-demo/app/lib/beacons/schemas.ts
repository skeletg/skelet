import { z } from "zod";

import { emailField, optionalText, requiredText } from "@/app/lib/forms/schema";

// Shared by the beacon forms (client validation) and their server actions, so no server imports here

export const backInStockSchema = z.object({
  productId: z.guid(),
  email: emailField(),
});

export const signupSchema = z.object({
  form: requiredText(),
  email: emailField(),
  name: optionalText(120),
});
