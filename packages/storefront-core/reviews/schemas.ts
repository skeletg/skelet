import { z } from "zod";

import { optionalText, requiredText } from "../forms/schema";

// Per-question answers post as answer.<questionId>.{inputType,rating,bool,text}, which Conform reads as a nested
// object. They stay strings here - the action turns them into the canonical GraphQL leaf, and required questions are
// the backend's to enforce
const answer = z.object({
  inputType: z.string(),
  rating: z.string().optional(),
  bool: z.string().optional(),
  text: z.string().optional(),
});

// A guest has to give a display name; a signed-in customer is named by their account
export function reviewSchema({ guest }: { guest: boolean }) {
  return z.object({
    productId: requiredText(),
    title: optionalText(140),
    body: optionalText(),
    guestName: guest ? requiredText().max(80, "tooLong") : optionalText(80),
    guestEmail: z.email({ error: "email" }).max(120, "tooLong").optional(),
    mediaAssetIds: optionalText(),
    answer: z.record(z.string(), answer).optional(),
  });
}
