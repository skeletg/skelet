"use client";

import { Link } from "@/app/components/LocaleLink";
import { useTranslations } from "next-intl";

import {
  Field,
  FormErrors,
  buttonPrimary,
  buttonSecondary,
} from "@/app/components/account/AccountUI";
import { loginAction, resendVerificationAction } from "@/app/lib/auth/actions";
import { loginSchema } from "@/app/lib/auth/schemas";
import { useActionForm } from "@/app/lib/forms/use-action-form";

export function LoginForm({
  returnTo,
  onSwitchView,
}: {
  returnTo?: string | null;
  onSwitchView?: (view: "register" | "forgot") => void;
}) {
  const t = useTranslations("auth.login");
  const tf = useTranslations("forms.fields");
  const { form, fields, formAction, onSubmit, pending } = useActionForm(
    loginAction,
    loginSchema,
  );

  const registerHref = returnTo
    ? `/account/register?returnTo=${encodeURIComponent(returnTo)}`
    : "/account/register";
  const notVerified = form.errors?.includes("emailNotVerified") ?? false;

  return (
    <>
      <form
        id={form.id}
        onSubmit={onSubmit}
        action={formAction}
        noValidate
        className="flex flex-col gap-4"
      >
        {returnTo ? (
          <input type="hidden" name="returnTo" value={returnTo} />
        ) : null}
        <Field
          label={tf("email")}
          field={fields.email}
          type="email"
          required
          autoComplete="email"
        />
        <Field
          label={tf("password")}
          field={fields.password}
          type="password"
          required
          autoComplete="current-password"
        />

        <FormErrors errors={form.errors} />

        <button
          type="submit"
          disabled={pending}
          className={`${buttonPrimary} w-full`}
        >
          {pending ? t("submitting") : t("submit")}
        </button>

        <p className="mt-1 text-center text-sm">
          {onSwitchView ? (
            <button
              type="button"
              onClick={() => onSwitchView("forgot")}
              className="text-muted-foreground hover:text-foreground hover:underline"
            >
              {t("forgot")}
            </button>
          ) : (
            <Link
              href="/account/forgot"
              className="text-muted-foreground hover:text-foreground hover:underline"
            >
              {t("forgot")}
            </Link>
          )}
        </p>

        <p className="mt-2 text-center text-sm text-muted-foreground">
          {t("newHere")}{" "}
          {onSwitchView ? (
            <button
              type="button"
              onClick={() => onSwitchView("register")}
              className="text-foreground underline underline-offset-2"
            >
              {t("createAccount")}
            </button>
          ) : (
            <Link
              href={registerHref}
              className="text-foreground underline underline-offset-2"
            >
              {t("createAccount")}
            </Link>
          )}
        </p>
      </form>
      {notVerified ? (
        <form action={resendVerificationAction} className="mt-3">
          <input type="hidden" name="email" value={fields.email.value ?? ""} />
          {returnTo ? (
            <input type="hidden" name="returnTo" value={returnTo} />
          ) : null}
          <button type="submit" className={`${buttonSecondary} w-full`}>
            {t("resendVerification")}
          </button>
        </form>
      ) : null}
    </>
  );
}
