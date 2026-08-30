"use client";

import { useState, useTransition } from "react";
import { Link } from "@/app/components/LocaleLink";

import { requestQuoteAction } from "@/app/lib/b2b/actions";

export function RequestQuoteForm() {
  const [open, setOpen] = useState(false);
  const [done, setDone] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();

  if (done) {
    return (
      <p className="glass-soft mt-4 rounded-lg p-3 text-sm">
        Quote requested - track it under{" "}
        <Link href="/account/company" className="underline underline-offset-2">
          Account → Company
        </Link>
        .
      </p>
    );
  }

  if (!open) {
    return (
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="text-sm text-muted-foreground underline-offset-2 hover:underline"
      >
        Buying in bulk? Request a quote →
      </button>
    );
  }

  return (
    <form
      action={(fd) =>
        startTransition(async () => {
          const res = await requestQuoteAction(fd);
          if (res.ok) setDone(true);
          else setError(res.error ?? "Something went wrong.");
        })
      }
      className="mt-4 flex max-w-md flex-col gap-2 rounded-lg border border-border p-3 text-sm"
    >
      <label className="flex flex-col gap-1">
        <span className="text-muted-foreground">
          Note to the store (quantities, timing, target price…)
        </span>
        <textarea
          name="note"
          rows={2}
          maxLength={500}
          className="rounded-lg border border-input bg-card px-3 py-2 outline-none focus:border-ring"
        />
      </label>
      {error && <p className="text-destructive">{error}</p>}
      <div className="flex gap-2">
        <button
          type="submit"
          disabled={pending}
          className="rounded-md bg-primary px-4 py-1.5 text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
        >
          {pending ? "Sending…" : "Request quote"}
        </button>
        <button
          type="button"
          onClick={() => setOpen(false)}
          className="rounded-md border border-border px-4 py-1.5 text-sm font-semibold transition-colors hover:bg-muted"
        >
          Cancel
        </button>
      </div>
    </form>
  );
}
