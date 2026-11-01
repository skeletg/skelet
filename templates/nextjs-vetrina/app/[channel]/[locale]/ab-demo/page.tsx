import type { Metadata } from "next";

import { LensTrack } from "@/app/components/LensTrack";
import { assignExperiment, forcedFor } from "@skelet/storefront-next/experiments/server";
import { AbHero } from "./AbHero";

export const metadata: Metadata = {
  title: "A/B demo",
  description: "A landing page split-tested with Prism.",
  robots: { index: false }, // demo page - keep it out of search.
};

const EXPERIMENT = "demo-hero-cta";
const CONTROL_CTA = "Shop products"; // fallback when the experiment isn't running

export default async function AbDemoPage({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const sp = await searchParams;
  const forced = forcedFor(EXPERIMENT, sp.prism);
  const assignment = await assignExperiment(EXPERIMENT, forced);

  const value = assignment?.value ?? CONTROL_CTA;
  const variation = assignment?.variation ?? "";
  const preview = assignment?.preview ?? false;

  return (
    <main className="flex flex-col">
      <AbHero value={value} variation={variation} preview={preview} />

      {/* Consent-gated, fired once - not for QA previews, forced views would pollute results */}
      {variation && !preview && (
        <LensTrack event="$experiment" props={{ experiment: EXPERIMENT, variation }} />
      )}

      <section className="border-b border-border">
        <div className="mx-auto w-full max-w-8xl px-6 md:px-10 py-14">
          <p className="text-xs uppercase tracking-wider text-muted-foreground">How it works</p>
          <h2 className="mt-1 text-2xl font-extrabold tracking-tight">Behind this page</h2>
          <div className="mt-6 grid gap-4 sm:grid-cols-3">
            <Step
              n="1"
              title="Assign"
              body="The server reads the visitor's id cookie (minted in middleware) and buckets them with an FNV hash - the same maths the client SDK and Go backend use, so everyone agrees. The right variation is in the first byte of HTML."
            />
            <Step
              n="2"
              title="Expose"
              body="Once the consent-gated tracker is live, a one-shot exposure event records which arm this visitor saw."
            />
            <Step
              n="3"
              title="Convert"
              body="Clicking the CTA fires the goal event (demo_cta_click). Prism counts goals that happen at or after a visitor's first exposure, per arm."
            />
          </div>
          <p className="mt-6 text-sm text-muted-foreground">
            See the live results in the dashboard under <span className="font-medium text-foreground">Signals → Prism</span>.
          </p>
        </div>
      </section>
    </main>
  );
}

function Step({ n, title, body }: { n: string; title: string; body: string }) {
  return (
    <div className="glass-soft rounded-xl p-5">
      <span className="flex h-7 w-7 items-center justify-center rounded-full bg-foreground text-xs font-semibold text-background">
        {n}
      </span>
      <h3 className="mt-3 text-base font-medium">{title}</h3>
      <p className="mt-2 text-sm leading-relaxed text-muted-foreground">{body}</p>
    </div>
  );
}
