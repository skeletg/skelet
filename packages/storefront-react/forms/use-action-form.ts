"use client";

import { useForm, type DefaultValue } from "@conform-to/react";
import { getZodConstraint } from "@conform-to/zod/v4";
import { startTransition, useActionState, useEffect, type FormEvent } from "react";
import type { z } from "zod";

import { parseForm, type FormResult } from "@skelet/storefront-core/forms/schema";

// Spelled out because the inferred type reaches into @conform-to/dom, which a published declaration can't name
type Conform<S extends z.ZodObject> = ReturnType<typeof useForm<z.input<S>, z.output<S>>>;

export type ActionForm<S extends z.ZodObject, R extends FormResult> = {
  form: Conform<S>[0];
  fields: Conform<S>[1];
  formAction: (formData: FormData) => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
  pending: boolean;
  lastResult: R | undefined;
};

// useActionState + Conform's useForm wired the one way every storefront form uses them: the server action returns a
// SubmissionResult, the same schema validates on the client first (on submit, then live as the shopper fixes a field),
// and the submitted values survive a failed round trip because the result carries them back. An action with
// more to say on success (the updated cart, an RMA number) returns a FormResult widened with it
//
// Forms bind onSubmit from here, not form.onSubmit. React 19 resets any form it submits through <form action>, and
// Conform answers that reset by rebuilding its state without the last result - so every field came back empty after
// a failed submit. Dispatching the action ourselves inside a transition skips React's reset; the action prop stays on
// the form so it still posts without JavaScript
export function useActionForm<
  S extends z.ZodObject,
  R extends FormResult = FormResult,
>(
  action: (prev: R | undefined, formData: FormData) => Promise<R>,
  schema: S,
  options: { defaultValue?: DefaultValue<z.input<S>> } = {},
): ActionForm<S, R> {
  const [lastResult, formAction, pending] = useActionState(action, undefined);
  const [form, fields] = useForm<z.input<S>, z.output<S>>({
    lastResult,
    defaultValue: options.defaultValue,
    constraint: getZodConstraint(schema),
    onValidate: ({ formData }) => parseForm(formData, schema),
    // Not onBlur: pressing Submit blurs the field being typed in, its error renders above the button, the button moves
    // before the mouse comes up - and the click that should have submitted lands on nothing
    shouldValidate: "onSubmit",
    shouldRevalidate: "onInput",
  });
  // The actions never echo a password back (hideFields), but with React's reset out of the way the typed one would
  // stay in its box - a failed sign-in or password change clears them, the rest of the form stays as typed
  //
  // The DOM value only: Conform's own value already lacks the password (the reply hid it), and its reset/update
  // intents would also clear the form-level error the shopper needs to read
  const formId = form.id;
  useEffect(() => {
    if (lastResult?.status !== "error") return;
    const el = document.getElementById(formId);
    if (!(el instanceof HTMLFormElement)) return;
    for (const input of el.querySelectorAll<HTMLInputElement>('input[type="password"]')) {
      input.value = "";
    }
  }, [lastResult, formId]);

  const onSubmit = (event: FormEvent<HTMLFormElement>) => {
    form.onSubmit(event); // client validation - an invalid form is prevented here
    if (event.defaultPrevented) return;
    event.preventDefault();
    const submitter = (event.nativeEvent as SubmitEvent).submitter;
    const formData = new FormData(event.currentTarget, submitter);
    startTransition(() => formAction(formData));
  };
  return { form, fields, formAction, onSubmit, pending, lastResult };
}
