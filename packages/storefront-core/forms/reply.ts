import type { Submission } from "@conform-to/zod/v4";

import { GraphQLRequestError } from "../graphql";
import type { FormResult } from "./schema";

// The backend's message when it sent one (validation and conflict text is shopper-actionable, and the front presenter
// already scrubs server faults), else a forms.errors key the form translates
export function errorMessage(err: unknown, fallbackKey: string): string {
  if (err instanceof GraphQLRequestError && err.errors[0]?.message) {
    return err.errors[0].message;
  }
  if (err instanceof Error && err.message) return err.message;
  return fallbackKey;
}

// A backend failure as a form-level error. The backend's errors name no input field, so there is nothing to attach
// to a field - hideFields keeps secrets (passwords) from being echoed back into the re-rendered form
export function replyError(
  submission: Submission<unknown, string[], unknown>,
  err: unknown,
  fallbackKey: string,
  hideFields?: string[],
): FormResult {
  return submission.reply({
    formErrors: [errorMessage(err, fallbackKey)],
    hideFields,
  });
}
