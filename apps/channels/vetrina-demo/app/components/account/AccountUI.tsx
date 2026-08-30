import type { ReactNode } from "react";
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

export function Field({
  label,
  name,
  type = "text",
  required,
  defaultValue,
  autoComplete,
  maxLength,
  placeholder,
  hint,
  className = "",
}: {
  label: string;
  name: string;
  type?: string;
  required?: boolean;
  defaultValue?: string;
  autoComplete?: string;
  maxLength?: number;
  placeholder?: string;
  hint?: string;
  className?: string;
}) {
  return (
    <label className={`flex flex-col gap-1.5 text-sm ${className}`}>
      <span className="font-medium">{label}</span>
      <input
        name={name}
        type={type}
        required={required}
        defaultValue={defaultValue}
        autoComplete={autoComplete}
        maxLength={maxLength}
        placeholder={placeholder}
        className={`${inputClass} w-full`}
      />
      {hint ? (
        <span className="text-xs text-muted-foreground">{hint}</span>
      ) : null}
    </label>
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
