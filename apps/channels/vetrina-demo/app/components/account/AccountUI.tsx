import type { ReactNode } from "react";
import { useTranslations } from "next-intl";
import { Link } from "@/app/components/LocaleLink";

export function AccountPageHeader({
  title,
  description,
  action,
}: {
  title: string;
  description?: string;
  action?: ReactNode;
}) {
  return (
    <header className="flex flex-wrap items-start justify-between gap-4 pb-6">
      <div>
        <h1 className="text-2xl font-extrabold tracking-tight sm:text-3xl">
          {title}
        </h1>
        {description ? (
          <p className="mt-1.5 max-w-prose text-sm text-muted-foreground">
            {description}
          </p>
        ) : null}
      </div>
      {action}
    </header>
  );
}

export function AccountCard({
  title,
  action,
  children,
  className = "",
  padded = true,
}: {
  title?: string;
  action?: ReactNode;
  children: ReactNode;
  className?: string;
  padded?: boolean;
}) {
  return (
    <section className={`glass rounded-xl ${padded ? "p-5" : ""} ${className}`}>
      {title ? (
        <div
          className={`flex items-center justify-between gap-3 ${padded ? "" : "px-5 pt-5"}`}
        >
          <h2 className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
            {title}
          </h2>
          {action}
        </div>
      ) : null}
      {children}
    </section>
  );
}

export function EmptyState({
  icon,
  title,
  description,
  actionHref,
  actionLabel,
}: {
  icon: ReactNode;
  title: string;
  description: string;
  actionHref?: string;
  actionLabel?: string;
}) {
  return (
    <div className="glass flex flex-col items-center rounded-xl px-6 py-14 text-center">
      <span className="flex size-12 items-center justify-center rounded-full bg-muted text-muted-foreground">
        {icon}
      </span>
      <p className="mt-4 font-semibold">{title}</p>
      <p className="mt-1 max-w-sm text-sm text-muted-foreground">
        {description}
      </p>
      {actionHref && actionLabel ? (
        <Link
          href={actionHref}
          className="mt-5 inline-flex h-10 items-center rounded-md bg-primary px-5 text-sm font-semibold text-primary-foreground"
        >
          {actionLabel}
        </Link>
      ) : null}
    </div>
  );
}

export function FieldRow({
  label,
  children,
}: {
  label: string;
  children: ReactNode;
}) {
  return (
    <div className="grid grid-cols-3 gap-4 py-3 text-sm">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="col-span-2">{children}</dd>
    </div>
  );
}

export function AuthCard({
  title,
  description,
  banner,
  children,
}: {
  title: string;
  description?: string;
  banner?: ReactNode;
  children: ReactNode;
}) {
  return (
    <div className="glass rounded-xl p-6 sm:p-8">
      <h1 className="text-2xl font-extrabold tracking-tight">{title}</h1>
      {description ? (
        <p className="mt-1.5 text-sm text-muted-foreground">{description}</p>
      ) : null}
      {banner ? <div className="mt-4">{banner}</div> : null}
      <div className="mt-6">{children}</div>
    </div>
  );
}

// The slice of a Conform FieldMetadata that Field reads - structural so this file, which server components import too,
// never pulls @conform-to/react (and its React context) into a server bundle
export type FieldMeta = {
  id: string;
  name: string;
  key?: string;
  initialValue?: unknown;
  errors?: string[];
  errorId: string;
  descriptionId: string;
};

export function Field({
  label,
  name,
  field,
  type = "text",
  required,
  defaultValue,
  autoComplete,
  maxLength,
  minLength,
  placeholder,
  hint,
  className = "",
}: {
  label: string;
  // Either a plain name, or a Conform field that brings its own name, value and errors
  name?: string;
  field?: FieldMeta;
  type?: string;
  required?: boolean;
  defaultValue?: string;
  autoComplete?: string;
  maxLength?: number;
  minLength?: number;
  placeholder?: string;
  hint?: string;
  className?: string;
}) {
  const errors = field?.errors?.length ? field.errors : null;
  const describedBy =
    [hint && field ? field.descriptionId : null, errors ? field!.errorId : null]
      .filter(Boolean)
      .join(" ") || undefined;
  // A Conform field takes its value from the form - the initial one, then whatever came back from a failed submit
  const value = field
    ? typeof field.initialValue === "string"
      ? field.initialValue
      : undefined
    : defaultValue;
  return (
    <label className={`flex flex-col gap-1.5 text-sm ${className}`}>
      <span className="font-medium">{label}</span>
      <input
        key={field?.key}
        id={field?.id}
        name={field?.name ?? name}
        type={type}
        required={required}
        defaultValue={value}
        autoComplete={autoComplete}
        maxLength={maxLength}
        minLength={minLength}
        placeholder={placeholder}
        aria-invalid={errors ? true : undefined}
        aria-describedby={describedBy}
        className={`${inputClass} w-full ${errors ? "border-red-400" : ""}`}
      />
      {hint ? (
        <span id={field?.descriptionId} className="text-xs text-muted-foreground">
          {hint}
        </span>
      ) : null}
      {errors ? <FieldErrors id={field!.errorId} errors={errors} /> : null}
    </label>
  );
}

// Zod messages are forms.errors keys; backend text isn't, and shows as sent
function useErrorText() {
  const t = useTranslations("forms.errors");
  return (msg: string) => (t.has(msg) ? t(msg) : msg);
}

export function FieldErrors({ id, errors }: { id?: string; errors: string[] }) {
  const text = useErrorText();
  return (
    <span id={id} className="text-xs text-red-700">
      {text(errors[0])}
    </span>
  );
}

export function FormError({ children }: { children: ReactNode }) {
  return (
    <p
      role="alert"
      className="rounded-lg border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-700"
    >
      {children}
    </p>
  );
}

// A Conform form's form-level errors (backend failures), translated when they are keys
export function FormErrors({ errors }: { errors?: string[] }) {
  const text = useErrorText();
  if (!errors?.length) return null;
  return <FormError>{text(errors[0])}</FormError>;
}

export function FormSuccess({ children }: { children: ReactNode }) {
  return (
    <p className="rounded-lg border border-emerald-300 bg-emerald-50 px-3 py-2 text-sm text-emerald-800">
      {children}
    </p>
  );
}

export const buttonPrimary =
  "inline-flex h-10 items-center justify-center rounded-md bg-primary px-5 text-sm font-semibold text-primary-foreground transition-opacity hover:opacity-90 disabled:opacity-50";

export const buttonSecondary =
  "inline-flex h-10 items-center justify-center rounded-md border border-border bg-background px-5 text-sm font-semibold transition-colors hover:bg-accent disabled:opacity-50";

// No width here on purpose: callers that want a narrow control (the loyalty redeem box, the quick-order qty) can't
// override a baked-in w-full, since both classes land in the same Tailwind layer and source order decides
export const inputClass =
  "rounded-lg border border-border bg-background px-3 py-2 text-sm outline-none transition-colors focus:border-foreground";
