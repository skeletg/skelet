"use client";

import { useEffect } from "react";
import { useTranslations } from "next-intl";
import { Link } from "@/app/components/LocaleLink";

export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  const t = useTranslations("errors");
  useEffect(() => {
    // Useful in dev; in prod a real telemetry hook would replace this
     
    console.error("storefront error boundary:", error);
  }, [error]);

  return (
    <main className="mx-auto flex min-h-[60vh] max-w-xl flex-col items-center justify-center gap-4 px-6 py-16 text-center">
      <p className="text-xs uppercase tracking-widest text-muted-foreground">
        {t("somethingWrong")}
      </p>
      <h1 className="text-3xl font-extrabold tracking-tight">
        {t("snag")}
      </h1>
      <p className="text-sm text-muted-foreground">
        {t("snagBody")}
      </p>
      {error.digest ? (
        <p className="font-mono text-xs text-muted-foreground">{t("ref", { digest: error.digest })}</p>
      ) : null}
      <div className="mt-4 flex gap-3">
        <button
          type="button"
          onClick={reset}
          className="inline-flex h-10 items-center rounded-md bg-primary px-5 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
        >
          {t("tryAgain")}
        </button>
        <Link
          href="/"
          className="inline-flex h-10 items-center rounded-md border border-border px-5 text-sm font-medium hover:bg-muted"
        >
          {t("goHome")}
        </Link>
      </div>
    </main>
  );
}
