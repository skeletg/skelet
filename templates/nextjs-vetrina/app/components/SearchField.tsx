"use client";

import { ArrowRightIcon, LoaderCircleIcon, SearchIcon, XIcon } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";
import { useRef, useState, useTransition } from "react";

import { localeHref, useDefaultLocale, useRouter } from "@/app/components/LocaleLink";

export function SearchField({ initialQuery, autoFocus }: { initialQuery: string; autoFocus?: boolean }) {
  const t = useTranslations("search");
  const router = useRouter();
  const locale = useLocale();
  const defaultLocale = useDefaultLocale();
  const input = useRef<HTMLInputElement>(null);
  const [value, setValue] = useState(initialQuery);
  const [shown, setShown] = useState(initialQuery);
  const [sending, startSending] = useTransition();
  // a search that only changes ?q keeps this mounted, so the box takes the new query from the page
  if (shown !== initialQuery) {
    setShown(initialQuery);
    setValue(initialQuery);
  }

  return (
    <form
      action={localeHref("/search", locale, defaultLocale)}
      method="GET"
      role="search"
      onSubmit={(e) => {
        e.preventDefault();
        const q = value.trim();
        if (!q) {
          input.current?.focus();
          return;
        }
        startSending(() => router.push(`/search?${new URLSearchParams({ q })}`));
      }}
      className="search-field group mt-5 flex h-14 w-full items-center rounded-full border border-border bg-card pl-5 pr-1.5 shadow-[0_1px_2px_oklch(0_0_0/0.04),0_10px_30px_-18px_oklch(0_0_0/0.25)] transition-[border-color,box-shadow] duration-200 focus-within:border-foreground/25 focus-within:shadow-[0_0_0_4px_color-mix(in_oklch,var(--ring)_16%,transparent),0_10px_30px_-18px_oklch(0_0_0/0.3)] md:h-16 md:pl-6"
    >
      <SearchIcon aria-hidden="true" className="size-5 shrink-0 text-muted-foreground transition-colors group-focus-within:text-foreground" />
      <input
        ref={input}
        type="search"
        name="q"
        value={value}
        onChange={(e) => setValue(e.target.value)}
        autoFocus={autoFocus}
        autoComplete="off"
        enterKeyHint="search"
        placeholder={t("placeholder")}
        aria-label={t("label")}
        className="h-full min-w-0 flex-1 bg-transparent px-3 text-base text-foreground outline-none placeholder:text-muted-foreground md:px-4 md:text-lg [&::-webkit-search-cancel-button]:appearance-none"
      />
      {value && (
        <button
          type="button"
          onClick={() => {
            setValue("");
            input.current?.focus();
          }}
          aria-label={t("clear")}
          className="mr-1 grid size-9 shrink-0 place-items-center rounded-full text-muted-foreground transition hover:bg-muted hover:text-foreground"
        >
          <XIcon aria-hidden="true" className="size-4" />
        </button>
      )}
      <button
        type="submit"
        aria-label={t("submit")}
        className="inline-flex h-11 shrink-0 items-center gap-2 rounded-full bg-primary px-4 text-sm font-semibold text-primary-foreground transition hover:bg-primary/90 active:scale-[0.97] sm:px-5 md:h-12 md:px-6"
      >
        <span className="hidden sm:inline">{t("submit")}</span>
        {sending ? (
          <LoaderCircleIcon aria-hidden="true" className="size-4 animate-spin" />
        ) : (
          <ArrowRightIcon aria-hidden="true" className="size-4 transition-transform group-focus-within:translate-x-0.5" />
        )}
      </button>
    </form>
  );
}
