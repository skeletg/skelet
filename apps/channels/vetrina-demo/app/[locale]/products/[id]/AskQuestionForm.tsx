"use client";

import { useState, useTransition } from "react";
import { MessageCircleQuestionIcon } from "lucide-react";

import { askQuestionAction } from "@/app/lib/qna/actions";
import { useCustomerIdentity } from "@/app/lib/customer/client";

const FIELD =
  "rounded-md border border-input bg-background px-3 py-2 text-sm outline-none transition focus:border-foreground";
const LABEL =
  "font-mono text-[11px] font-medium uppercase tracking-[0.08em] text-muted-foreground";

export function AskQuestionForm({ productId }: { productId: string }) {
  // Resolved after hydration - the page is cached HTML, so the server render has no session
  // Unresolved reads as anonymous, which just shows the name/email fields the form needs anyway
  const signedIn = useCustomerIdentity().identity != null;
  const [open, setOpen] = useState(false);
  const [done, setDone] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();

  if (done) {
    return (
      <p className="rounded-md border border-emerald-500/30 bg-emerald-500/10 px-4 py-3 text-sm">
        Thanks - your question was sent to the store team. It will appear here
        once answered.
      </p>
    );
  }

  if (!open) {
    return (
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="inline-flex h-10 items-center gap-2 rounded-md border border-foreground/25 px-5 text-sm font-semibold transition hover:border-foreground"
      >
        <MessageCircleQuestionIcon className="h-4 w-4" aria-hidden="true" />
        Ask a question
      </button>
    );
  }

  return (
    <form
      action={(fd) =>
        startTransition(async () => {
          const res = await askQuestionAction(productId, fd);
          if (res.ok) setDone(true);
          else setError(res.error ?? "Something went wrong.");
        })
      }
      className="glass flex max-w-xl flex-col gap-4 rounded-lg p-5"
    >
      <label className="flex flex-col gap-1.5">
        <span className={LABEL}>Your question</span>
        <textarea
          name="question"
          required
          minLength={10}
          maxLength={1000}
          rows={3}
          autoFocus
          className={FIELD}
          placeholder="What would you like to know about this product?"
        />
      </label>
      {!signedIn && (
        <div className="grid gap-4 sm:grid-cols-2">
          <label className="flex flex-col gap-1.5">
            <span className={LABEL}>Display name</span>
            <input name="authorName" required maxLength={120} className={FIELD} />
          </label>
          <label className="flex flex-col gap-1.5">
            <span className={LABEL}>Email</span>
            <input name="authorEmail" type="email" className={FIELD} />
            <span className="text-xs text-muted-foreground">
              Never shown publicly
            </span>
          </label>
        </div>
      )}
      {error && <p className="text-sm text-destructive">{error}</p>}
      <div className="flex gap-2">
        <button
          type="submit"
          disabled={pending}
          className="inline-flex h-10 items-center rounded-md bg-primary px-5 text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/85 disabled:opacity-50"
        >
          {pending ? "Sending…" : "Submit question"}
        </button>
        <button
          type="button"
          onClick={() => setOpen(false)}
          className="inline-flex h-10 items-center rounded-md border border-foreground/25 px-5 text-sm font-semibold transition hover:border-foreground"
        >
          Cancel
        </button>
      </div>
    </form>
  );
}
