"use client";

import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { ArrowRightIcon } from "lucide-react";
import { useTranslations } from "next-intl";

import { productHref } from "@skelet/storefront-core/entity-href";
import { OVERVIEW_EVENTS, overviewNamed, overviewSegments, type OverviewExchange, type SearchOverview } from "@skelet/storefront-core/search/overview";
import type { ProductSummary } from "@skelet/storefront-next/catalog/read";
import { overviewCardsAction } from "@skelet/storefront-next/search/actions";
import { LensTrack } from "@/app/components/LensTrack";
import { Link } from "@/app/components/LocaleLink";
import { preloadPanel as preload } from "@/app/components/assistant/AssistantDock";
import { AssistantOrb } from "@/app/components/assistant/AssistantOrb";
import { useAssistant } from "@/app/components/assistant/AssistantProvider";
import { AssistantSpark } from "@/app/components/assistant/AssistantSpark";

const track = (event: string, props?: Record<string, unknown>) => window.lens?.track?.(event, props);

// The overview's room eases to whatever it ends up holding - a longer answer than the placeholder, or none at all
export function OverviewSlot({ children }: { children: ReactNode }) {
  const outer = useRef<HTMLDivElement>(null);
  const inner = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const room = outer.current;
    const held = inner.current;
    if (!room || !held || typeof ResizeObserver === "undefined") return;
    const fit = new ResizeObserver(() => {
      room.style.height = `${held.offsetHeight}px`;
    });
    fit.observe(held);
    return () => fit.disconnect();
  }, []);
  return (
    <div ref={outer} aria-live="polite" className="search-overview-slot">
      <div ref={inner} className="flow-root">
        {children}
      </div>
    </div>
  );
}

function Frame({ busy = false, children }: { busy?: boolean; children: ReactNode }) {
  const t = useTranslations("assistant.overview");
  const label = useId();
  return (
    <section aria-labelledby={label} aria-busy={busy || undefined} data-busy={busy ? "" : undefined} className="search-overview mt-8 max-w-4xl md:mt-10">
      <h2 id={label} className="flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">
        {busy ? <AssistantOrb size={16} busy /> : <AssistantSpark className="size-4 text-signal" />}
        {t("label")}
      </h2>
      {children}
    </section>
  );
}

export function OverviewPending() {
  const t = useTranslations("assistant.overview");
  return (
    <Frame busy>
      <div className="mt-4 space-y-3">
        <p className="assistant-shimmer text-lg leading-relaxed md:text-xl">{t("writing")}</p>
        <span aria-hidden="true" className="search-overview__line block h-4 w-full" />
        <span aria-hidden="true" className="search-overview__line block h-4 w-10/12" />
      </div>
      <div aria-hidden="true" className="mt-6 h-9" />
    </Frame>
  );
}

export function OverviewBox({ overview, chat }: { overview: SearchOverview; chat: OverviewExchange }) {
  const t = useTranslations("assistant.overview");
  const segments = overviewSegments(overview.text, overview.products);
  return (
    <Frame>
      <LensTrack key={chat.question} event={OVERVIEW_EVENTS.shown} props={{ query: chat.question, products: overviewNamed(segments, overview.products).length }} />
      <p className="mt-4 text-lg leading-relaxed text-foreground/90 md:text-xl md:leading-relaxed">
        {segments.map((s, i) =>
          s.kind === "text" ? (
            s.text
          ) : (
            <Link
              key={i}
              href={productHref(s.product)}
              onClick={() => track(OVERVIEW_EVENTS.click, { query: chat.question, position: s.position })}
              className="font-semibold text-foreground underline decoration-signal/60 decoration-2 underline-offset-[5px] transition-colors hover:text-signal hover:decoration-signal"
            >
              {s.product.name}
            </Link>
          ),
        )}
      </p>
      <div className="mt-6 flex flex-wrap items-center gap-x-5 gap-y-3">
        <ContinueInChat chat={chat} />
        <p className="text-xs text-muted-foreground">{t("disclaimer")}</p>
      </div>
    </Frame>
  );
}

function ContinueInChat({ chat }: { chat: OverviewExchange }) {
  const { enabled, continueWith } = useAssistant();
  const t = useTranslations("assistant.overview");
  const cards = useRef<Promise<ProductSummary[]> | null>(null);
  const [opening, setOpening] = useState(false);
  if (!enabled) return null;
  // started on the way to the button, so a click rarely waits; a failed fetch is tried again next time
  const fetchCards = () =>
    (cards.current ??=
      chat.productIds.length === 0
        ? Promise.resolve([])
        : overviewCardsAction(chat.productIds).catch(() => {
            cards.current = null;
            return [];
          }));
  const prepare = () => {
    preload();
    void fetchCards();
  };
  return (
    <button
      type="button"
      onClick={async () => {
        if (opening) return;
        track(OVERVIEW_EVENTS.chat, { query: chat.question });
        setOpening(true);
        const products = await fetchCards();
        setOpening(false);
        continueWith({ question: chat.question, reply: chat.reply, recap: chat.recap, products });
      }}
      onPointerEnter={prepare}
      onFocus={prepare}
      aria-busy={opening || undefined}
      className="group/chat inline-flex h-9 shrink-0 items-center gap-2 rounded-full bg-foreground/10 pl-3.5 pr-4 text-sm font-medium text-foreground transition-colors hover:bg-foreground/15 aria-busy:opacity-70"
    >
      <AssistantSpark className="size-4 text-signal" />
      {t("continue")}
      <ArrowRightIcon className="size-3.5 transition-transform group-hover/chat:translate-x-0.5" strokeWidth={1.8} aria-hidden="true" />
    </button>
  );
}
