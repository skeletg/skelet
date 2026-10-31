"use client";

import { useState } from "react";
import { CheckCircle2Icon } from "lucide-react";

import { trackGoal } from "@skelet/storefront-next/experiments/client";

export function AbHero({
  value,
  variation,
  preview,
}: {
  value: string;
  variation: string;
  preview: boolean;
}) {
  const [converted, setConverted] = useState(false);
  const arm = variation === "control" ? "control" : variation ? "variant" : "-";

  function onConvert() {
    trackGoal("demo_cta_click", { source: "ab-demo" });
    setConverted(true);
  }

  return (
    <section className="relative overflow-hidden border-b border-border">
      <div
        aria-hidden="true"
        className="pointer-events-none absolute inset-0 bg-gradient-to-br from-emerald-500/10 via-background/40 to-background/80"
      />
      <div className="relative mx-auto flex w-full max-w-8xl flex-col items-start gap-6 px-6 md:px-10 py-20 sm:py-28">
        <span className="glass inline-flex items-center gap-2 rounded-md px-3 py-1 text-xs uppercase tracking-wider text-muted-foreground">
          <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" />
          Prism A/B test · demo-hero-cta{preview ? " · preview" : ""}
        </span>
        <h1 className="max-w-3xl text-4xl font-semibold leading-tight tracking-tight text-foreground sm:text-5xl">
          One headline, two calls to action.
        </h1>
        <p className="max-w-xl text-base leading-relaxed text-muted-foreground sm:text-lg">
          The button below is split-tested. Each visitor is bucketed by a
          deterministic hash of their browser id, sees one variation, and sticks
          to it on every reload. Clicking it records a conversion for that arm.
        </p>
        <div className="flex flex-wrap items-center gap-3">
          <button
            type="button"
            onClick={onConvert}
            className="inline-flex h-11 items-center gap-2 rounded-md bg-foreground px-6 text-sm font-medium text-background transition hover:opacity-90"
          >
            {value}
          </button>
          {converted && (
            <span className="inline-flex items-center gap-1.5 text-sm font-medium text-emerald-600">
              <CheckCircle2Icon className="h-4 w-4" />
              Conversion tracked
            </span>
          )}
        </div>
        <p className="text-xs text-muted-foreground">
          You&apos;re seeing the <span className="font-medium text-foreground">{arm}</span> arm. Append{" "}
          <code className="rounded bg-muted px-1 py-0.5 text-[11px]">?prism=demo-hero-cta:variant</code> to this URL to
          preview the other one (preview clicks aren&apos;t counted).
        </p>
      </div>
    </section>
  );
}
