"use client";

import { useMemo } from "react";
import { useTranslations } from "next-intl";

import {
  Field,
  FormErrors,
  buttonPrimary,
} from "@/app/components/account/AccountUI";
import { resetPasswordAction } from "@skelet/storefront-next/auth/actions";
import { resetSchema } from "@skelet/storefront-next/auth/schemas";
import { useActionForm } from "@skelet/storefront-next/forms/use-action-form";

export function ResetForm({ token }: { token: string }) {
  // Bound, not a hidden field - a hidden input is captured by rrweb (maskAllInputs has
  // no entry for type=hidden), which would ship a live reset token into session replay
  const boundAction = useMemo(
    () => resetPasswordAction.bind(null, token),
    [token],
  );
  const t = useTranslations("auth.reset");
  const tf = useTranslations("forms.fields");
  const tc = useTranslations("common");
  const { form, fields, formAction, onSubmit, pending } = useActionForm(
    boundAction,
    resetSchema,
  );

  return (
    <form
      id={form.id}
      onSubmit={onSubmit}
      action={formAction}
      noValidate
      className="flex flex-col gap-4"
    >
      <Field
        label={t("newPassword")}
        field={fields.newPassword}
        type="password"
        required
        minLength={8}
        autoComplete="new-password"
      />
      <Field
        label={tf("confirmPassword")}
        field={fields.confirmPassword}
        type="password"
        required
        minLength={8}
        autoComplete="new-password"
      />

      <FormErrors errors={form.errors} />

      <button
        type="submit"
        disabled={pending}
        className={`${buttonPrimary} w-full`}
      >
        {pending ? tc("saving") : t("submit")}
      </button>
    </form>
  );
}
