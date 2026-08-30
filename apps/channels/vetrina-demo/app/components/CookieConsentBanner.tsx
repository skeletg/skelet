"use client";

import { Link } from "@/app/components/LocaleLink";

import {
  acceptAllConsentAction,
  essentialOnlyConsentAction,
} from "@/app/lib/consent/actions";
import { useConsentDecided } from "@/app/lib/consent/use-consent";

// Resolved in the browser - a server-side cookie read here would force every route dynamic, and
// the banner has to work on a page served identically to every visitor
export function CookieConsentBanner() {
  if (useConsentDecided()) return null;

  return (
    <div
      // Positioning: fixed bottom so the banner doesn't push page content
      className="fixed inset-x-0 bottom-0 z-40 border-t border-border bg-card/95 px-4 py-4 shadow-lg backdrop-blur"
      role="region"
      aria-label="Cookie consent"
    >
      <div className="mx-auto flex max-w-8xl flex-col items-start gap-3 text-sm sm:flex-row sm:items-center sm:justify-between">
        <p className="text-foreground">
          We use cookies for the essentials (sign in, cart, language)
          plus optional analytics and marketing. Pick &ldquo;Essential
          only&rdquo; to skip the optional ones.{" "}
          <Link
            href="/privacy"
            className="underline underline-offset-2 hover:no-underline"
          >
            Privacy policy
          </Link>
          .
        </p>
        <div className="flex flex-wrap gap-2">
          <form action={essentialOnlyConsentAction}>
            <button
              type="submit"
              className="inline-flex h-9 items-center rounded-md border border-border px-4 text-sm font-medium hover:bg-muted"
            >
              Essential only
            </button>
          </form>
          <form action={acceptAllConsentAction}>
            <button
              type="submit"
              className="inline-flex h-9 items-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90"
            >
              Accept all
            </button>
          </form>
        </div>
      </div>
    </div>
  );
}
