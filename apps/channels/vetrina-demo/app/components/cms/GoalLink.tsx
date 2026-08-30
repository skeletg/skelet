"use client";

import { Link } from "@/app/components/LocaleLink";
import type { ReactNode } from "react";
export function GoalLink({
  href,
  goalEvent,
  className,
  children,
}: {
  href: string;
  goalEvent?: string;
  className?: string;
  children: ReactNode;
}) {
  return (
    <Link
      href={href}
      className={className}
      onClick={
        goalEvent
          ? () => {
              window.lens?.track?.(goalEvent, { source: "cms_cta" });
              window.lens?.flush?.();
            }
          : undefined
      }
    >
      {children}
    </Link>
  );
}
