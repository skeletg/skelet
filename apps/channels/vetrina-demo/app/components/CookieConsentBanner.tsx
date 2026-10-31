"use client";

import { useEffect, useState } from "react";
import { Link } from "@/app/components/LocaleLink";
import { useTranslations } from "next-intl";

import {
  acceptAllConsentAction,
  essentialOnlyConsentAction,
  saveConsentAction,
} from "@/app/lib/consent/actions";
import { NONE_GRANTED, type Consent } from "@/app/lib/consent/categories";
import { readClientConsent } from "@/app/lib/consent/client";
import { useConsentDecided } from "@/app/lib/consent/use-consent";

// Fired by the footer's "Cookie settings" - changing your mind has to be as easy as the first choice
export const OPEN_CONSENT_EVENT = "consent:open";

const CATEGORIES = ["analytics", "ads", "personalization"] as const;

// Resolved in the browser - a server-side cookie read here would force every route dynamic, and
// the banner has to work on a page served identically to every visitor
export function CookieConsentBanner() {
  const t = useTranslations("consent");
  const decided = useConsentDecided();
  const [reopened, setReopened] = useState(false);
  const [customizing, setCustomizing] = useState<Consent | null>(null); // the boxes' starting state while open

  useEffect(() => {
    const open = () => {
      setReopened(true);
      setCustomizing(readClientConsent());
    };
    window.addEventListener(OPEN_CONSENT_EVENT, open);
    return () => window.removeEventListener(OPEN_CONSENT_EVENT, open);
  }, []);

  if (decided && !reopened) return null;

  // Each choice closes the banner here too - a reopened banner has no "undecided" state to hide it
  const close = (action: (form: FormData) => Promise<void>) => async (form: FormData) => {
    await action(form);
    setReopened(false);
    setCustomizing(null);
  };

  return (
    <div
      // Positioning: fixed bottom so the banner doesn't push page content
      className="fixed inset-x-0 bottom-0 z-40 border-t border-border bg-card/95 px-4 py-4 shadow-lg backdrop-blur"
      role="region"
      aria-label={t("label")}
    >
      <div className="mx-auto flex max-w-8xl flex-col gap-3 text-sm">
        <div className="flex flex-col items-start gap-3 sm:flex-row sm:items-center sm:justify-between">
          <p className="text-foreground">
            {t("body")}{" "}
            <Link
              href="/privacy"
              className="underline underline-offset-2 hover:no-underline"
            >
              {t("policy")}
            </Link>
            .
          </p>
          <div className="flex flex-wrap gap-2">
            {!customizing && (
              <button
                type="button"
                onClick={() => setCustomizing(NONE_GRANTED)}
                className="inline-flex h-9 items-center rounded-md border border-border px-4 text-sm font-medium hover:bg-muted"
              >
                {t("customize")}
              </button>
            )}
            <form action={close(essentialOnlyConsentAction)}>
              <button
                type="submit"
                className="inline-flex h-9 items-center rounded-md border border-border px-4 text-sm font-medium hover:bg-muted"
              >
                {t("essentialOnly")}
              </button>
            </form>
            <form action={close(acceptAllConsentAction)}>
              <button
                type="submit"
                className="inline-flex h-9 items-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90"
              >
                {t("acceptAll")}
              </button>
            </form>
          </div>
        </div>
        {customizing && (
          <form action={close(saveConsentAction)} className="flex flex-col gap-3 border-t border-border pt-3">
            <fieldset className="flex flex-col gap-2">
              <legend className="sr-only">{t("customize")}</legend>
              <label className="flex items-start gap-2">
                <input type="checkbox" checked disabled className="mt-0.5" />
                <span>
                  <span className="font-medium">{t("necessary")}</span>
                  <span className="block text-muted-foreground">{t("necessaryHelp")}</span>
                </span>
              </label>
              {CATEGORIES.map((c) => (
                <label key={c} className="flex items-start gap-2">
                  <input type="checkbox" name={c} defaultChecked={customizing[c]} className="mt-0.5" />
                  <span>
                    <span className="font-medium">{t(c)}</span>
                    <span className="block text-muted-foreground">{t(`${c}Help`)}</span>
                  </span>
                </label>
              ))}
            </fieldset>
            <div>
              <button
                type="submit"
                className="inline-flex h-9 items-center rounded-md border border-border px-4 text-sm font-medium hover:bg-muted"
              >
                {t("save")}
              </button>
            </div>
          </form>
        )}
      </div>
    </div>
  );
}

// The footer's way back in - a client island, so the footer itself stays a server component
export function CookieSettingsButton({ label }: { label: string }) {
  return (
    <button
      type="button"
      onClick={() => window.dispatchEvent(new Event(OPEN_CONSENT_EVENT))}
      className="uppercase transition hover:text-foreground"
    >
      {label}
    </button>
  );
}
