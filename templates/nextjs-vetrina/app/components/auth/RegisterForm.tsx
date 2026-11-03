"use client";

import { Link } from "@/app/components/LocaleLink";
import { useTranslations } from "next-intl";

import {
  Field,
  FormErrors,
  buttonPrimary,
} from "@/app/components/account/AccountUI";
import { registerAction } from "@skelet/storefront-next/auth/actions";
import { registerSchema } from "@skelet/storefront-core/auth/schemas";
import { useActionForm } from "@skelet/storefront-react/forms/use-action-form";

export function RegisterForm({
  returnTo,
  referralCode,
  onSwitchView,
}: {
  returnTo?: string | null;
  referralCode?: string | null;
  onSwitchView?: (view: "login") => void;
}) {
  const t = useTranslations("auth.register");
  const tf = useTranslations("forms.fields");
  const tc = useTranslations("common");
  const { form, fields, formAction, onSubmit, pending } = useActionForm(
    registerAction,
    registerSchema,
  );

  const loginHref = returnTo
    ? `/account/login?returnTo=${encodeURIComponent(returnTo)}`
    : "/account/login";

  return (
    <form
      id={form.id}
      onSubmit={onSubmit}
      action={formAction}
      noValidate
      className="flex flex-col gap-3"
    >
      {returnTo ? (
        <input type="hidden" name="returnTo" value={returnTo} />
      ) : null}
      {referralCode ? (
        <input type="hidden" name="referralCode" value={referralCode} />
      ) : null}
      <div className="grid grid-cols-2 gap-4">
        <Field
          label={tf("firstName")}
          field={fields.firstName}
          required
          autoComplete="given-name"
        />
        <Field
          label={tf("lastName")}
          field={fields.lastName}
          required
          autoComplete="family-name"
        />
      </div>
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
        autoComplete="new-password"
        minLength={8}
        hint={tf("passwordHint")}
      />
      <Field
        label={tf("confirmPassword")}
        field={fields.confirmPassword}
        type="password"
        required
        autoComplete="new-password"
        minLength={8}
      />
      <label className="flex items-center gap-2.5 text-sm">
        <input
          type="checkbox"
          key={fields.marketingOptIn.key}
          name={fields.marketingOptIn.name}
          defaultChecked={fields.marketingOptIn.initialValue === "on"}
          className="size-4 rounded border-border"
        />
        {t("marketingOptIn")}
      </label>

      <FormErrors errors={form.errors} />

      <button
        type="submit"
        disabled={pending}
        className={`${buttonPrimary} w-full`}
      >
        {pending ? t("submitting") : t("submit")}
      </button>

      <p className="mt-2 text-center text-sm text-muted-foreground">
        {t("haveAccount")}{" "}
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
            href={loginHref}
            className="text-foreground underline underline-offset-2"
          >
            {tc("signIn")}
          </Link>
        )}
      </p>
    </form>
  );
}
