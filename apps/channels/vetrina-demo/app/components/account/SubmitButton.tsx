"use client";

// The account area is full of bare <form action={serverAction}> triggers. This is the submit for them: disabled while
// the action is in flight (no double-submit), and with an optional arm-then-confirm step for anything destructive

import { useState, type ReactNode } from "react";
import { useFormStatus } from "react-dom";

export function SubmitButton({
  className,
  children,
  pendingLabel = "Working…",
  confirmLabel,
  cancelLabel = "Keep",
}: {
  className: string;
  children: ReactNode;
  pendingLabel?: string;
  // Set to turn the first click into "are you sure" - the second click submits
  confirmLabel?: string;
  cancelLabel?: string;
}) {
  const { pending } = useFormStatus();
  const [armed, setArmed] = useState(false);

  if (confirmLabel && !armed && !pending) {
    return (
      <button
        type="button"
        className={className}
        onClick={() => setArmed(true)}
      >
        {children}
      </button>
    );
  }

  const submit = (
    <button
      type="submit"
      disabled={pending}
      aria-busy={pending}
      className={`${className} disabled:opacity-50`}
    >
      {pending ? pendingLabel : (confirmLabel ?? children)}
    </button>
  );

  if (!confirmLabel) return submit;

  return (
    <span className="inline-flex items-center gap-2">
      {submit}
      {!pending && (
        <button
          type="button"
          onClick={() => setArmed(false)}
          className="text-xs text-muted-foreground hover:text-foreground"
        >
          {cancelLabel}
        </button>
      )}
    </span>
  );
}
