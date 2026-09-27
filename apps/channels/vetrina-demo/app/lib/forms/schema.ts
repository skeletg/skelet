import { parseWithZod, type SubmissionResult } from "@conform-to/zod/v4";
import { z } from "zod";

// Shared by the client (onValidate) and the server action, so no "use server" and no server imports here
//
// Every message is a key under forms.errors in messages/en.json - Field and FormErrors translate them, and anything
// that isn't a key (backend text) renders verbatim. Conform coerces an empty input to undefined before zod sees it,
// so "missing" surfaces as an invalid_type issue - hence the input check rather than a min(1) alone

export type FormResult = SubmissionResult<string[]>;

export function parseForm<S extends z.ZodType>(formData: FormData, schema: S) {
  return parseWithZod(formData, { schema });
}

const missingOr = (key: string) => (iss: { input?: unknown }) =>
  iss.input === undefined ? "required" : key;

export const requiredText = () =>
  z.string({ error: "required" }).trim().min(1, "required");

// Blank becomes null, which is what the GraphQL inputs take for "clear this column"
export const optionalText = () =>
  z
    .string()
    .trim()
    .optional()
    .transform((v) => v || null);

export const emailField = () => z.email({ error: missingOr("email") });

// Required but unconstrained - a sign-in or current-password box must accept whatever the account was created with
export const passwordField = () => z.string({ error: "required" });

export const newPasswordField = () =>
  z.string({ error: "required" }).min(8, "passwordMin");

