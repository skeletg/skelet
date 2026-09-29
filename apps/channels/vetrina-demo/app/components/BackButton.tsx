"use client";

// "Back" control - returns to wherever the shopper actually came from, which is often not the parent in the breadcrumb
// trail (search results, a rail on the homepage, a collection)


import { ChevronLeftIcon } from "lucide-react";
import { useTranslations } from "next-intl";
import { useRouter } from "@/app/components/LocaleLink";

export function BackButton({
  fallbackHref = "/products",
  label,
  className = "",
}: {
  fallbackHref?: string;
  label?: string;
  className?: string;
}) {
  const router = useRouter();
  const t = useTranslations("common");

  // Going back is only safe when the previous entry is ours - otherwise the shopper
  // gets bounced off the site (or to about:blank in a fresh tab)
  const canGoBack = () => {
    try {
      const ref = document.referrer;
      if (ref && new URL(ref).origin === window.location.origin) return true;
    } catch {
    }
    return window.history.length > 2;
  };

  return (
    <button
      type="button"
      onClick={() => {
        if (canGoBack()) router.back();
        else router.push(fallbackHref);
      }}
      className={`inline-flex items-center gap-1 text-sm text-muted-foreground transition-colors hover:text-foreground ${className}`}
    >
      <ChevronLeftIcon className="size-4" aria-hidden="true" />
      {label ?? t("back")}
    </button>
  );
}
