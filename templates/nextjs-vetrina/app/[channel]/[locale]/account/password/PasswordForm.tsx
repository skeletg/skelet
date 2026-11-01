"use client";

import { useTranslations } from "next-intl";

import {
  Field,
  FormErrors,
  FormSuccess,
  buttonPrimary,
} from "@/app/components/account/AccountUI";
import { changePasswordAction } from "@skelet/storefront-next/auth/actions";
import { changePasswordSchema } from "@skelet/storefront-core/auth/schemas";
import { useActionForm } from "@skelet/storefront-react/forms/use-action-form";

export function PasswordForm() {
  const t = useTranslations("account.password");
  const tf = useTranslations("forms.fields");
  const tc = useTranslations("common");
  const { form, fields, formAction, onSubmit, pending, lastResult } = useActionForm(
    changePasswordAction,
    changePasswordSchema,
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
        label={t("current")}
        field={fields.currentPassword}
        type="password"
        required
        autoComplete="current-password"
      />
      <Field
        label={t("new")}
        field={fields.newPassword}
        type="password"
        required
        autoComplete="new-password"
        minLength={8}
        hint={tf("passwordHint")}
      />
      <Field
        label={t("confirm")}
        field={fields.confirmPassword}
        type="password"
        required
        autoComplete="new-password"
        minLength={8}
      />

      <FormErrors errors={form.errors} />
      {lastResult?.status === "success" && (
        <FormSuccess>{t("updated")}</FormSuccess>
      )}

      <div className="mt-1">
        <button type="submit" disabled={pending} className={buttonPrimary}>
          {pending ? tc("saving") : t("submit")}
        </button>
      </div>
    </form>
  );
}
