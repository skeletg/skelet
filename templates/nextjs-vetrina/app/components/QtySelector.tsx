"use client";

import { useId, useState, type ChangeEvent } from "react";
import { useTranslations } from "next-intl";

const SIZE_STYLES = {
  sm: {
    wrap: "h-8 rounded-md text-xs",
    button: "h-8 w-7 text-sm",
    input: "h-8 w-10 text-xs",
  },
  md: {
    wrap: "h-11 rounded-md text-sm [[data-stuck-buy]_&]:overflow-hidden [[data-stuck-buy]_&]:rounded-full",
    button: "h-11 w-10 text-base",
    input: "h-11 w-12 text-sm",
  },
  // Matches AddToCartButton size="lg" (h-14) so the stepper lines up with the oversized
  // headline CTA variant
  lg: {
    wrap: "h-14 rounded-md text-base",
    button: "h-14 w-12 text-lg",
    input: "h-14 w-14 text-base",
  },
} as const;

export function QtySelector({
  name,
  defaultValue,
  value: controlledValue,
  onValueChange,
  min,
  max = 99,
  step = 1,
  decimals = 0,
  disabled,
  size = "md",
  label,
  invalid = false,
}: {
  name?: string;
  defaultValue?: number;
  value?: number;
  onValueChange?: (next: number) => void;
  min?: number;
  max?: number;
  step?: number;
  decimals?: number;
  disabled?: boolean;
  size?: "sm" | "md" | "lg";
  label?: string;
  invalid?: boolean;
}) {
  const id = useId();
  const t = useTranslations("common");
  const styles = SIZE_STYLES[size];
  const effectiveMin = min ?? step;
  const initial = defaultValue ?? step;
  // Controlled when the parent supplies value + onValueChange; otherwise the component
  // owns its state and submits via the name-d input
  const controlled = controlledValue !== undefined && onValueChange !== undefined;
  const [internal, setInternal] = useState(() => clampStep(initial, effectiveMin, max, step, decimals));
  const value = controlled
    ? clampStep(controlledValue as number, effectiveMin, max, step, decimals)
    : internal;

  const set = (v: number) => {
    const c = clampStep(v, effectiveMin, max, step, decimals);
    if (controlled) onValueChange!(c);
    else setInternal(c);
  };
  const dec = () => set(value - step);
  const inc = () => set(value + step);

  const onChange = (e: ChangeEvent<HTMLInputElement>) => {
    const raw = e.target.value;
    if (raw === "") {
      // Allow the field to be temporarily empty while editing - commit on blur
      set(effectiveMin);
      return;
    }
    const n = Number(raw);
    if (Number.isFinite(n)) set(n);
  };

  return (
    <div className="flex items-center gap-2">
      {label ? (
        <label
          htmlFor={id}
          className="text-xs font-medium uppercase tracking-wider text-muted-foreground"
        >
          {label}
        </label>
      ) : null}
      <div
        className={`inline-flex items-center overflow-hidden border ${
          invalid ? "border-primary/70 bg-primary/5" : "border-border"
        } ${styles.wrap} ${disabled ? "opacity-50" : ""}`}
      >
        <button
          type="button"
          onClick={dec}
          disabled={disabled || value <= effectiveMin}
          aria-label={t("decreaseQty")}
          className={`${styles.button} flex items-center justify-center text-foreground transition-colors hover:bg-muted disabled:cursor-not-allowed disabled:text-muted-foreground`}
        >
          −
        </button>
        <input
          id={id}
          name={name}
          type="number"
          inputMode={decimals > 0 ? "decimal" : "numeric"}
          min={effectiveMin}
          max={max}
          step={step}
          value={decimals > 0 ? value.toFixed(decimals) : value}
          onChange={onChange}
          disabled={disabled}
          className={`${styles.input} appearance-none border-x border-border bg-transparent text-center tabular-nums outline-none [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none`}
        />
        <button
          type="button"
          onClick={inc}
          disabled={disabled || value >= max}
          aria-label={t("increaseQty")}
          className={`${styles.button} flex items-center justify-center text-foreground transition-colors hover:bg-muted disabled:cursor-not-allowed disabled:text-muted-foreground`}
        >
          +
        </button>
      </div>
    </div>
  );
}

function clampStep(n: number, min: number, max: number, step: number, decimals: number): number {
  if (!Number.isFinite(n)) n = min;
  if (step <= 0) step = 1;
  // Snap to the nearest multiple of step (relative to min so a min=0.5 step=0.5 stepper
  // accepts 0.5, 1.0, 1.5 not 0, 0.5, 1)
  const offset = n - min;
  const snapped = min + Math.round(offset / step) * step;
  const clamped = Math.min(max, Math.max(min, snapped));
  if (decimals > 0) {
    const f = Math.pow(10, decimals);
    return Math.round(clamped * f) / f;
  }
  return Math.trunc(clamped);
}
