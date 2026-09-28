"use client";

import { Link } from "@/app/components/LocaleLink";
import { useTranslations } from "next-intl";

import {
  Field,
  FormErrors,
  buttonPrimary,
} from "@/app/components/account/AccountUI";
import { requestPasswordResetAction } from "@/app/lib/auth/actions";
import { forgotSchema } from "@/app/lib/auth/schemas";
import { useActionForm } from "@/app/lib/forms/use-action-form";

export function ForgotForm({
  onSwitchView,
}: {
  onSwitchView?: (view: "login") => void;
} = {}) {
  const t = useTranslations("auth.forgot");
  const tf = useTranslations("forms.fields");
  const tc = useTranslations("common");
  const { form, fields, formAction, pending, lastResult } = useActionForm(
    requestPasswordResetAction,
    forgotSchema,
  );

  // After the mutation succeeds we render a static success message in place of the form
  // - even if the email doesn't exist, the backend returns ok and we stay quiet about
  // it (anti-enumeration)
  if (lastResult?.status === "success") {
    return (
      <div className="rounded-lg border border-border bg-muted p-4 text-sm">
        <p>{t("sent")}</p>
        <p className="mt-3 text-muted-foreground">
          {onSwitchView ? (
            <button
              type="button"
              onClick={() => onSwitchView("login")}
              className="text-foreground underline underline-offset-2"
            >
              {t("backToSignIn")}
            </button>
          ) : (
            <Link
              href="/account/login"
              className="text-foreground underline underline-offset-2"
            >
              {t("backToSignIn")}
            </Link>
          )}
        </p>
      </div>
    );
  }

  return (
    <form
      id={form.id}
      onSubmit={form.onSubmit}
      action={formAction}
      noValidate
      className="flex flex-col gap-4"
    >
      <Field
        label={tf("email")}
        field={fields.email}
        type="email"
        required
        autoComplete="email"
      />

      <FormErrors errors={form.errors} />

      <button
        type="submit"
        disabled={pending}
        className={`${buttonPrimary} w-full`}
      >
        {pending ? t("submitting") : t("submit")}
      </button>

      <p className="mt-2 text-center text-sm text-muted-foreground">
        {t("remembered")}{" "}
        {onSwitchView ? (
          <button
            type="button"
            onClick={() => onSwitchView("login")}
            className="text-foreground underline underline-offset-2"
          >
            {tc("signIn")}
          </button>
        ) : (
          <Link
            href="/account/login"
            className="text-foreground underline underline-offset-2"
          >
            {tc("signIn")}
          </Link>
        )}
      </p>
    </form>
  );
}
