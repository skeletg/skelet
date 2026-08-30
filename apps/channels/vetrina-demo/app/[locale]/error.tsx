"use client";

import { useEffect } from "react";
import { Link } from "@/app/components/LocaleLink";

export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    // Useful in dev; in prod a real telemetry hook would replace this
     
    console.error("storefront error boundary:", error);
  }, [error]);

  return (
    <main className="mx-auto flex min-h-[60vh] max-w-xl flex-col items-center justify-center gap-4 px-6 py-16 text-center">
      <p className="text-xs uppercase tracking-widest text-muted-foreground">
        Something went wrong
      </p>
      <h1 className="text-3xl font-extrabold tracking-tight">
        We hit a snag.
      </h1>
      <p className="text-sm text-muted-foreground">
        Try again in a moment. If the problem keeps happening, please contact
        support and share the reference code below.
      </p>
      {error.digest ? (
        <p className="font-mono text-xs text-muted-foreground">ref: {error.digest}</p>
      ) : null}
      <div className="mt-4 flex gap-3">
        <button
          type="button"
          onClick={reset}
          className="inline-flex h-10 items-center rounded-md bg-primary px-5 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
        >
          Try again
        </button>
        <Link
          href="/"
          className="inline-flex h-10 items-center rounded-md border border-border px-5 text-sm font-medium hover:bg-muted"
        >
          Go home
        </Link>
      </div>
    </main>
  );
}
