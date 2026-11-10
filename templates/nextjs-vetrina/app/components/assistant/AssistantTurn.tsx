"use client";

import { memo } from "react";
import { CheckIcon, InfoIcon, TriangleAlertIcon } from "lucide-react";
import { useTranslations } from "next-intl";

import { ProductCard } from "@/app/components/ProductCard";
import { Rail, RailDots, RailProvider } from "@/app/components/Rail";

import { AssistantLinks } from "./AssistantLinks";
import { useAssistant } from "./AssistantProvider";
import { AssistantText } from "./AssistantText";
import { ProductSpotlight } from "./ProductSpotlight";
import { showedSomething, type Turn } from "./turns";

type Props = {
  index: number;
  turn: Turn;
  arrived: boolean;
  // The newest reply offers its follow-ups, once nothing else is on its way
  offerFollowUps: boolean;
  storeOrigins: string[];
  onAsk: (text: string) => void;
  onOpenCart: () => void;
  // On a phone the drawer covers the page, so "See it" closes it onto the list the reply moved
  onSeeList?: () => void;
};

// What a reply says in words when it has none: an apology only when it showed nothing either
export function replyText(turn: Turn, noAnswer: string): string {
  return turn.content !== "" || turn.incomplete || showedSomething(turn) ? turn.content : noAnswer;
}

export const UserTurn = memo(function UserTurn({ text }: { text: string }) {
  const t = useTranslations("assistant.speaker");
  return (
    <div className="flex justify-end">
      <p className="max-w-[85%] whitespace-pre-wrap rounded-2xl rounded-br-sm bg-muted px-3 py-2">
        <span className="sr-only">{t("you")}: </span>
        {text}
      </p>
    </div>
  );
});

export const AssistantTurn = memo(function AssistantTurn({ index, turn, arrived, offerFollowUps, storeOrigins, onAsk, onOpenCart, onSeeList }: Props) {
  const t = useTranslations("assistant");
  const { name } = useAssistant();
  const text = replyText(turn, t("noAnswer"));
  return (
    <div data-turn={index} className={`space-y-3 ${arrived ? "assistant-arrive" : ""}`}>
      <span className="sr-only">{name || t("speaker.assistant")}:</span>
      {text ? <AssistantText text={text} storeOrigins={storeOrigins} /> : null}
      {turn.incomplete ? (
        <p className="flex items-start gap-2 text-xs text-muted-foreground">
          <InfoIcon className="mt-px size-3.5 shrink-0" aria-hidden="true" />
          {t("cutShort")}
        </p>
      ) : null}
      {turn.detail ? <ProductSpotlight detail={turn.detail} /> : null}
      {turn.navigated ? (
        <p className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
          <CheckIcon className="size-3.5 text-signal" aria-hidden="true" />
          {t("links.updated", { count: turn.navigated.count ?? 0 })}
          {onSeeList ? (
            <button type="button" onClick={onSeeList} className="font-semibold text-foreground underline underline-offset-2">
              {t("links.see")}
            </button>
          ) : null}
        </p>
      ) : null}
      {turn.links?.length ? <AssistantLinks links={turn.links} /> : null}
      {turn.products?.length ? (
        <RailProvider>
          <Rail label={t("products")} wheel={false}>
            {turn.products.map((p) => (
              <div key={p.id} className="w-36 shrink-0 snap-start">
                <ProductCard product={p} variant="compact" />
              </div>
            ))}
          </Rail>
          <RailDots />
        </RailProvider>
      ) : null}
      {turn.cartLost || turn.cartMissed ? (
        <p className="flex items-start gap-2 text-xs text-muted-foreground">
          <TriangleAlertIcon className="mt-px size-3.5 shrink-0 text-signal" aria-hidden="true" />
          {turn.cartLost ? t("cartLost") : t("cartMissed", { items: turn.cartMissed?.join(", ") ?? "" })}
        </p>
      ) : null}
      {turn.showCart ? (
        <button
          type="button"
          onClick={onOpenCart}
          className="flex h-10 w-full items-center justify-center rounded-md bg-primary text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/85"
        >
          {t("reviewCart")}
        </button>
      ) : null}
      {offerFollowUps && turn.followUps?.length ? (
        <div className="flex flex-wrap gap-2 pt-1" aria-label={t("followUps")} role="group">
          {turn.followUps.map((followUp, i) => (
            <button
              key={i}
              type="button"
              onClick={() => onAsk(followUp)}
              className="rounded-full border border-border px-3 py-1.5 text-left text-xs transition hover:border-foreground/30 hover:bg-muted"
            >
              {followUp}
            </button>
          ))}
        </div>
      ) : null}
    </div>
  );
});
