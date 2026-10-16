// Prism's assignment rule, shared by the client SDK and server-side assignment so both put a visitor in
// the same arm - FNV-1a over seed + anon id, the same bucketing as the Go backend

import { fnv1a } from "@/app/lib/analytics/visitor";

export type Variation = { key: string; weight: number; value: string };
export type Experiment = { key: string; coverage: number; goalEvent: string; variations: Variation[] };

function bucket(seed: string, anonID: string): number {
  return (fnv1a(seed + anonID) % 10000) / 10000;
}

export function assign(exp: Experiment, anonID: string): Variation | null {
  if (exp.coverage <= 0 || exp.variations.length === 0) return null;
  if (exp.coverage < 100 && bucket(`${exp.key}:coverage`, anonID) >= exp.coverage / 100) return null;
  const total = exp.variations.reduce((a, v) => a + Math.max(0, v.weight), 0);
  if (total <= 0) return exp.variations[0];
  const target = bucket(exp.key, anonID) * total;
  let acc = 0;
  for (const v of exp.variations) {
    acc += Math.max(0, v.weight);
    if (target < acc) return v;
  }
  return exp.variations[exp.variations.length - 1];
}
