import type { ReactNode } from "react";

// A radio drawn as a bordered card - variant, delivery and payment pickers all use it
type Props = {
  name: string;
  value: string;
  checked?: boolean;
  disabled?: boolean;
  // Submit the form through this button whenever the pick changes (enhance.ts)
  autosubmit?: string;
  children?: ReactNode;
};

export function Choice({ name, value, checked = false, disabled = false, autosubmit, children }: Props) {
  return (
    <label className="flex cursor-pointer items-center gap-3 rounded-xl border border-border bg-card px-4 py-3 transition-colors has-checked:border-foreground has-checked:ring-1 has-checked:ring-ring has-disabled:cursor-not-allowed has-disabled:opacity-50">
      <input type="radio" name={name} value={value} required disabled={disabled} defaultChecked={checked} data-autosubmit={autosubmit} className="size-4 min-h-0 w-auto shrink-0 accent-primary" />
      {children}
    </label>
  );
}
