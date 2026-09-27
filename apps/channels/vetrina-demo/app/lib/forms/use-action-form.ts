"use client";

import { useForm } from "@conform-to/react";
import { getZodConstraint } from "@conform-to/zod/v4";
import { useActionState } from "react";
import type { z } from "zod";

import { parseForm, type FormResult } from "@/app/lib/forms/schema";

// useActionState + Conform's useForm wired the one way every storefront form uses them: the server action returns a
// SubmissionResult, the same schema validates on the client first (on blur, then on input once a field has been
// touched), and the submitted values survive a failed round trip because the result carries them back
export function useActionForm<S extends z.ZodObject>(
  action: (prev: FormResult | undefined, formData: FormData) => Promise<FormResult>,
  schema: S,
  options: { defaultValue?: Partial<Record<keyof z.input<S>, string>> } = {},
) {
  const [lastResult, formAction, pending] = useActionState(action, undefined);
  const [form, fields] = useForm<z.input<S>, z.output<S>>({
    lastResult,
    defaultValue: options.defaultValue as never,
    constraint: getZodConstraint(schema),
    onValidate: ({ formData }) => parseForm(formData, schema),
    shouldValidate: "onBlur",
    shouldRevalidate: "onInput",
  });
  return { form, fields, formAction, pending, lastResult };
}
