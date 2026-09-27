"use client";

import { useForm, type DefaultValue } from "@conform-to/react";
import { getZodConstraint } from "@conform-to/zod/v4";
import { useActionState } from "react";
import type { z } from "zod";

import { parseForm, type FormResult } from "@/app/lib/forms/schema";

// useActionState + Conform's useForm wired the one way every storefront form uses them: the server action returns a
// SubmissionResult, the same schema validates on the client first (on blur, then on input once a field has been
// touched), and the submitted values survive a failed round trip because the result carries them back. An action with
// more to say on success (the updated cart, an RMA number) returns a FormResult widened with it
export function useActionForm<
  S extends z.ZodObject,
  R extends FormResult = FormResult,
>(
  action: (prev: R | undefined, formData: FormData) => Promise<R>,
  schema: S,
  options: { defaultValue?: DefaultValue<z.input<S>> } = {},
) {
  const [lastResult, formAction, pending] = useActionState(action, undefined);
  const [form, fields] = useForm<z.input<S>, z.output<S>>({
    lastResult,
    defaultValue: options.defaultValue,
    constraint: getZodConstraint(schema),
    onValidate: ({ formData }) => parseForm(formData, schema),
    shouldValidate: "onBlur",
    shouldRevalidate: "onInput",
  });
  return { form, fields, formAction, pending, lastResult };
}
