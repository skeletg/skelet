"use client";

import type { ReactNode } from "react";

import { useExperimentAssignment } from "@/app/lib/experiments/client";

// The page is cached HTML, so the arm can't be chosen on the server - every variant is
// rendered up front and this picks one after hydration, which also fires the exposure
export function CmsVariantPicker({
  experiment,
  control,
  variants,
}: {
  experiment: string;
  control: ReactNode;
  variants: { key: string; node: ReactNode }[];
}) {
  const variation = useExperimentAssignment(experiment)?.variation;
  const picked = variation
    ? variants.find((v) => v.key === variation)
    : undefined;
  return <>{picked ? picked.node : control}</>;
}
