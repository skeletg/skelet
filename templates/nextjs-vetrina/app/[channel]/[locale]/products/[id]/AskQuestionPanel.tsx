"use client";

import { useMemo } from "react";
import { useTranslations } from "next-intl";

import { FieldErrors, FormErrors } from "@/app/components/account/AccountUI";
import { askQuestionAction } from "@skelet/storefront-next/qna/actions";
import { askQuestionSchema } from "@skelet/storefront-core/qna/schemas";
import { useCustomerIdentity } from "@skelet/storefront-next/customer/client";
import { useActionForm } from "@skelet/storefront-react/forms/use-action-form";

const FIELD =
  "rounded-md border border-input bg-background px-3 py-2 text-sm outline-none transition focus:border-foreground";
const LABEL =
  "font-mono text-[11px] font-medium uppercase tracking-[0.08em] text-muted-foreground";

// The form half of AskQuestionForm, split off so zod and Conform download on the shopper's click rather
// than with every product page
export function AskQuestionPanel({
  productId,
  onCancel,
}: {
  productId: string;
  onCancel: () => void;
}) {
  // Resolved after hydration - the page is cached HTML, so the server render has no session
  // Unresolved reads as anonymous, which just shows the name/email fields the form needs anyway
  const signedIn = useCustomerIdentity().identity != null;
  const t = useTranslations("product.qna");
  const tf = useTranslations("forms.fields");
  const tc = useTranslations("common");
  const boundAction = useMemo(
    () => askQuestionAction.bind(null, productId),
    [productId],
  );
  const schema = useMemo(
    () => askQuestionSchema({ guest: !signedIn }),
    [signedIn],
  );
  const { form, fields, formAction, onSubmit, pending, lastResult } = useActionForm(
    boundAction,
    schema,
  );

  if (lastResult?.status === "success") {
    return (
      <p className="rounded-md border border-emerald-500/30 bg-emerald-500/10 px-4 py-3 text-sm">
        {t("thanks")}
      </p>
    );
  }

  return (
    <form
      id={form.id}
      onSubmit={onSubmit}
      action={formAction}
      noValidate
      className="glass flex max-w-xl flex-col gap-4 rounded-lg p-5"
    >
      <label className="flex flex-col gap-1.5">
        <span className={LABEL}>{t("yourQuestion")}</span>
        <textarea
          key={fields.question.key}
          name={fields.question.name}
          defaultValue={fields.question.initialValue ?? ""}
          required
          minLength={10}
          maxLength={1000}
          rows={3}
          autoFocus
          aria-invalid={fields.question.errors ? true : undefined}
          aria-describedby={
            fields.question.errors ? fields.question.errorId : undefined
          }
          className={FIELD}
          placeholder={t("placeholder")}
        />
        {fields.question.errors ? (
          <FieldErrors
            id={fields.question.errorId}
            errors={fields.question.errors}
          />
        ) : null}
      </label>
      {!signedIn && (
        <div className="grid gap-4 sm:grid-cols-2">
          <label className="flex flex-col gap-1.5">
            <span className={LABEL}>{t("displayName")}</span>
            <input
              key={fields.authorName.key}
              name={fields.authorName.name}
              defaultValue={fields.authorName.initialValue ?? ""}
              required
              maxLength={120}
              aria-invalid={fields.authorName.errors ? true : undefined}
              aria-describedby={
                fields.authorName.errors ? fields.authorName.errorId : undefined
              }
              className={FIELD}
            />
            {fields.authorName.errors ? (
              <FieldErrors
                id={fields.authorName.errorId}
                errors={fields.authorName.errors}
              />
            ) : null}
          </label>
          <label className="flex flex-col gap-1.5">
            <span className={LABEL}>{tf("email")}</span>
            <input
              key={fields.authorEmail.key}
              name={fields.authorEmail.name}
              defaultValue={fields.authorEmail.initialValue ?? ""}
              type="email"
              aria-invalid={fields.authorEmail.errors ? true : undefined}
              aria-describedby={
                fields.authorEmail.errors
                  ? fields.authorEmail.errorId
                  : fields.authorEmail.descriptionId
              }
              className={FIELD}
            />
            {fields.authorEmail.errors ? (
              <FieldErrors
                id={fields.authorEmail.errorId}
                errors={fields.authorEmail.errors}
              />
            ) : (
              <span
                id={fields.authorEmail.descriptionId}
                className="text-xs text-muted-foreground"
              >
                {t("neverShown")}
              </span>
            )}
          </label>
        </div>
      )}
      <FormErrors errors={form.errors} />
      <div className="flex gap-2">
        <button
          type="submit"
          disabled={pending}
          className="inline-flex h-10 items-center rounded-md bg-primary px-5 text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/85 disabled:opacity-50"
        >
          {pending ? tc("sending") : t("submit")}
        </button>
        <button
          type="button"
          onClick={onCancel}
          className="inline-flex h-10 items-center rounded-md border border-foreground/25 px-5 text-sm font-semibold transition hover:border-foreground"
        >
          {tc("cancel")}
        </button>
      </div>
    </form>
  );
}
