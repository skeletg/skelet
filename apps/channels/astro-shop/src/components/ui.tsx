import type { ButtonHTMLAttributes, ReactNode } from "react";

// The few shared primitives - everything renders on the server, none of it hydrates

const base =
  "inline-flex min-h-12 items-center justify-center gap-2 rounded-full px-6 font-semibold transition duration-200 ease-(--ease-out-soft) active:scale-[0.97] disabled:cursor-not-allowed disabled:opacity-40";

const variants = {
  solid: "bg-primary text-primary-foreground hover:bg-primary/85",
  ghost: "border border-border bg-card text-foreground hover:border-foreground",
};

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & { variant?: keyof typeof variants };

export function Button({ variant = "solid", className = "", ...rest }: ButtonProps) {
  return <button className={`${base} ${variants[variant]} ${className}`} {...rest} />;
}

export function ButtonLink({ href, children, variant = "solid", className = "" }: { href: string; children?: ReactNode; variant?: keyof typeof variants; className?: string }) {
  return (
    <a href={href} className={`${base} ${variants[variant]} ${className}`}>
      {children}
    </a>
  );
}

export function Notice({ children }: { children?: ReactNode }) {
  return (
    <p role="alert" className="mb-6 rounded-xl border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive">
      {children}
    </p>
  );
}
