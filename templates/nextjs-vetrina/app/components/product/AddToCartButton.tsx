"use client";

import { useFormStatus } from "react-dom";
import { useTranslations } from "next-intl";

const SIZE_STYLES = {
  lg: "h-14 gap-2.5 rounded-md px-8 text-base",
  md: "h-12 gap-2 rounded-md px-7 text-sm",
  sm: "h-10 gap-1.5 rounded-md px-5 text-xs",
} as const;

export function AddToCartButton({
  disabled,
  blocked,
  onBlockedClick,
  size = "md",
  form,
  stickyPrice,
  className = "",
  children,
}: {
  // Hard disable: genuinely can't buy (out of stock, uploading)
  disabled?: boolean;
  // Soft block: the buy is only waiting on a shopper choice (unpicked variant /
  // required option / nothing selected)
  blocked?: boolean;
  onBlockedClick?: () => void;
  size?: "lg" | "md" | "sm";
  // Associates the button with a form it doesn't sit inside (grouped's sticky bar) -
  // note useFormStatus can't see that form, so the bar button skips the Adding… state
  form?: string;
  // Shown beside the label, but only while inside a StickyBuyRow bar - in the page
  // the big price sits right above the button already
  stickyPrice?: string;
  className?: string;
  children: React.ReactNode;
}) {
  const { pending } = useFormStatus();
  const t = useTranslations("product");
  // The drawer opens (on success only) and errors surface via AddToCartFeedback, driven by
  // the form's useActionState result - opening it on the pending falling edge instead would
  // fire even when the add was rejected
  // drawer on the pending falling edge (which fired even when the add was rejected)
  const isDisabled = disabled || pending;
  // Soft-blocked only applies when not hard-disabled/pending
  const softBlocked = !!blocked && !isDisabled;
  const looksDisabled = isDisabled || softBlocked;
  const sizeClass = SIZE_STYLES[size];
  const lookClass = looksDisabled
    ? "cursor-not-allowed bg-muted text-muted-foreground shadow-none"
    : "bg-primary text-primary-foreground hover:bg-primary/85";

  return (
    <button
      type="submit"
      form={form}
      disabled={isDisabled}
      aria-disabled={softBlocked || undefined}
      aria-busy={pending}
      onClick={
        softBlocked
          ? (e) => {
              e.preventDefault();
              onBlockedClick?.();
            }
          : undefined
      }
      className={`${sizeClass} ${className} ${lookClass} inline-flex items-center justify-center font-semibold transition-colors duration-200 [[data-stuck-buy]_&]:rounded-full`}
    >
      {pending && (
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth={2.5}
          strokeLinecap="round"
          className={`${size === "sm" ? "h-3 w-3" : size === "lg" ? "h-5 w-5" : "h-4 w-4"} animate-spin`}
          aria-hidden="true"
        >
          <path d="M12 3a9 9 0 1 0 9 9" />
        </svg>
      )}
      {pending ? t("adding") : children}
      {!pending && stickyPrice && (
        <span className="hidden font-mono text-sm font-medium tabular-nums opacity-90 [[data-stuck-buy]_&]:inline">
          {stickyPrice}
        </span>
      )}
    </button>
  );
}
