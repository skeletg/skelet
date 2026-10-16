import { z } from "zod";

import { emailField, requiredText } from "@/app/lib/forms/schema";

export const trackOrderSchema = z.object({
  number: requiredText(),
  email: emailField(),
  postalCode: requiredText(),
});
