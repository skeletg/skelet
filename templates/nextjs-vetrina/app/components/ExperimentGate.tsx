"use client";

import type { ReactNode } from "react";

import { useExperiment } from "@skelet/storefront-next/experiments/client";

// Assigns client-side - server assignment would pin one arm onto the shared cache entry
export function ExperimentGate({
  experiment,
  value,
  children,
}: {
  experiment: string;
  value: string;
  children: ReactNode;
}) {
  return useExperiment(experiment) === value ? <>{children}</> : null;
}
