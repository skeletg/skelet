import { z } from "zod";

import { optionalText, requiredText } from "../forms/schema";

// A guest has to say who's asking; a signed-in customer is named by their account. The client knows which after
// hydration, the action from the session cookie
export function askQuestionSchema({ guest }: { guest: boolean }) {
  return z.object({
    question: requiredText()
      .min(10, "questionTooShort")
      .max(1000, "tooLong"),
    authorName: guest ? requiredText().max(120, "tooLong") : optionalText(120),
    authorEmail: z.email({ error: "email" }).optional(),
  });
}
