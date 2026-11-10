"use client";

import { useEffect, useRef } from "react";
import { useLocale, useTranslations } from "next-intl";

import type { AssistantPage, AssistantStep } from "@skelet/storefront-core/assistant/types";

import { AssistantOrb } from "./AssistantOrb";
import { AssistantTurn, UserTurn, replyText } from "./AssistantTurn";
import { stepLine } from "./steps";
import type { Turn } from "./turns";
import type { Problem } from "./useAssistantChat";

// Opening suggestions by the kind of page the shopper is on - anything else gets the general ones
const STARTERS = {
  product: ["starters.product.1", "starters.product.2", "starters.product.3"],
  listing: ["starters.listing.1", "starters.listing.2", "starters.listing.3"],
  search: ["starters.search.1", "starters.search.2", "starters.search.3"],
  page: ["starters.page.1", "starters.page.2"],
} as const;
const GENERAL_STARTERS = ["starter1", "starter2", "starter3"] as const;
const REPLY_GAP = 16;
type StarterKey = (typeof STARTERS)[keyof typeof STARTERS][number] | (typeof GENERAL_STARTERS)[number];

function startersFor(page: AssistantPage | null): readonly StarterKey[] {
  return page && Object.hasOwn(STARTERS, page.kind) ? STARTERS[page.kind as keyof typeof STARTERS] : GENERAL_STARTERS;
}

type Props = {
  turns: Turn[];
  sending: boolean;
  step?: AssistantStep | null;
  problem: Problem | null;
  arrived: number;
  storeName: string;
  pageInfo: AssistantPage | null;
  storeOrigins: string[];
  open: boolean;
  onAsk: (text: string) => void;
  onRetry: () => void;
  onOpenCart: () => void;
  onSeeList?: () => void;
};

export function AssistantThread({ turns, sending, step = null, problem, arrived, storeName, pageInfo, storeOrigins, open, onAsk, onRetry, onOpenCart, onSeeList }: Props) {
  const t = useTranslations("assistant");
  const locale = useLocale();
  const threadRef = useRef<HTMLDivElement>(null);
  const working = stepLine(step, locale);
  const latest = turns[arrived];
  // Read out only the words of a reply as it lands - the cards and buttons under it are there to look at
  const announce = latest?.role === "assistant" ? [replyText(latest, t("noAnswer")), latest.incomplete ? t("cutShort") : ""].filter(Boolean).join(" ") : "";

  const landed = !sending && !problem && arrived >= 0 && arrived === turns.length - 1;
  useEffect(() => {
    const el = threadRef.current;
    if (!el) return;
    const reduce = window.matchMedia?.("(prefers-reduced-motion: reduce)").matches;
    const reply = landed ? el.querySelector<HTMLElement>(`[data-turn="${arrived}"]`) : null;
    const top = reply ? el.scrollTop + reply.getBoundingClientRect().top - el.getBoundingClientRect().top - REPLY_GAP : el.scrollHeight;
    el.scrollTo({ top, behavior: reduce ? "auto" : "smooth" });
  }, [turns, sending, open, landed, arrived]);

  return (
    <div ref={threadRef} className="assistant-scroll min-h-0 flex-1 space-y-4 overflow-y-auto px-4 py-4 text-sm">
      <p aria-live="polite" className="sr-only">
        {announce}
      </p>
      {turns.length === 0 ? (
        <div className="flex min-h-full flex-col">
          <div className="flex flex-col items-center pt-12 text-center">
            <AssistantOrb size={104} busy={sending} rest={0.55} />
            <h2 className="mt-12 text-2xl font-extrabold tracking-tight">{t("greeting")}</h2>
            <p className="mt-1.5 max-w-sm text-muted-foreground">{storeName ? t("intro", { store: storeName }) : t("introHere")}</p>
          </div>
          <div className="mt-auto flex flex-col items-start gap-2 pt-8">
            {startersFor(pageInfo).map((key) => {
              const text = t(key, { title: pageInfo?.title ?? "" });
              return (
                <button key={key} type="button" onClick={() => onAsk(text)} className="rounded-full border border-border px-3.5 py-1.5 text-left transition hover:border-foreground/30 hover:bg-muted">
                  {text}
                </button>
              );
            })}
          </div>
        </div>
      ) : (
        turns.map((turn, i) =>
          turn.role === "user" ? (
            <UserTurn key={i} text={turn.content} />
          ) : (
            <AssistantTurn
              key={i}
              index={i}
              turn={turn}
              arrived={i === arrived}
              offerFollowUps={i === turns.length - 1 && !sending}
              storeOrigins={storeOrigins}
              onAsk={onAsk}
              onOpenCart={onOpenCart}
              onSeeList={onSeeList}
            />
          ),
        )
      )}
      {sending ? (
        <p className="flex items-center gap-2">
          <AssistantOrb size={16} busy />
          <span key={working?.key ?? "thinking"} className="assistant-shimmer">
            {working ? t(working.key, { detail: working.detail }) : t("thinking")}
          </span>
        </p>
      ) : null}
      {problem ? (
        <div role="alert" className="flex flex-wrap items-center gap-x-3 gap-y-1 rounded-md border border-amber-300/60 bg-amber-50 px-3 py-2 text-amber-800 dark:bg-amber-950/40 dark:text-amber-300">
          <span>{t(`problems.${problem.kind}`)}</span>
          {problem.kind === "invalid" ? null : (
            <button type="button" onClick={onRetry} className="font-semibold underline underline-offset-2">
              {t("retry")}
            </button>
          )}
        </div>
      ) : null}
    </div>
  );
}
