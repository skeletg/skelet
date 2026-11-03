import type { ReactNode } from "react";

// A labelled form control - the control itself is the children
type Props = {
  label: string;
  optional?: boolean;
  // Spans both columns of a two-column form grid
  wide?: boolean;
  children?: ReactNode;
};

export function Field({ label, optional = false, wide = false, children }: Props) {
  return (
    <label className={`grid gap-1 text-sm font-medium${wide ? " sm:col-span-2" : ""}`}>
      <span>
        {label}
        {optional && <span className="font-normal text-muted-foreground"> (optional)</span>}
      </span>
      {children}
    </label>
  );
}
