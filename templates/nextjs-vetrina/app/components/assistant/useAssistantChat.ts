"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useLocale } from "next-intl";

import { sendToAssistant } from "@skelet/storefront-core/assistant/client";
import { isStorePath, pageForAssistant } from "@skelet/storefront-core/assistant/page";
import { MAX_MESSAGE_CHARS, type AssistantAnswer, type AssistantProblem, type AssistantStep } from "@skelet/storefront-core/assistant/types";
import { useCartStore } from "@skelet/storefront-next/cart/store";
import { useRouter } from "@/app/components/LocaleLink";

import { assistantHref } from "./AssistantLinks";
import { CONVERSATION_KEY, newChatId, readChatId, readTurns, toMessages, withExchange, type Exchange, type Turn } from "./turns";

export const MAX_INPUT = MAX_MESSAGE_CHARS;

export type Problem = { kind: AssistantProblem; text: string };

function here(): string {
  return window.location.pathname + window.location.search;
}

export function useAssistantChat() {
  const router = useRouter();
  const locale = useLocale();
  const { hydrate } = useCartStore();
  const [turns, setTurns] = useState<Turn[] | null>(null);
  const [sending, setSending] = useState(false);
  const [step, setStep] = useState<AssistantStep | null>(null);
  const [problem, setProblem] = useState<Problem | null>(null);
  const [arrived, setArrived] = useState(-1);
  const current = useRef<Turn[]>([]);
  const chatId = useRef("");
  const generation = useRef(0);
  const inFlight = useRef<AbortController | null>(null);
  const queued = useRef<string | null>(null);
  const held = useRef<Exchange | null>(null);
  const askRef = useRef<(text: string) => Promise<boolean>>(async () => false);

  const commit = useCallback((next: Turn[]) => {
    current.current = next;
    setTurns(next);
  }, []);

  const carryOn = useCallback(
    (exchange: Exchange) => {
      const next = withExchange(current.current, exchange);
      if (next === current.current) return;
      setProblem(null);
      setArrived(next.length - 1);
      commit(next);
    },
    [commit],
  );

  const releaseHeld = useCallback(() => {
    const exchange = held.current;
    held.current = null;
    if (exchange) carryOn(exchange);
  }, [carryOn]);

  useEffect(() => {
    let restored: Turn[] = [];
    let saved: string | null = null;
    try {
      saved = sessionStorage.getItem(CONVERSATION_KEY);
      restored = readTurns(saved);
    } catch {
      // storage blocked - the chat just starts empty
    }
    chatId.current = readChatId(saved);
    // eslint-disable-next-line react-hooks/set-state-in-effect
    commit(restored);
  }, [commit]);

  useEffect(() => {
    if (!turns) return;
    try {
      sessionStorage.setItem(CONVERSATION_KEY, JSON.stringify({ turns, id: chatId.current }));
    } catch {
      // private mode or a full quota - the conversation just won't survive a reload
    }
  }, [turns]);

  useEffect(
    () => () => {
      generation.current += 1;
      inFlight.current?.abort();
    },
    [],
  );

  // Resolves false when the turn failed and its words should go back to the shopper
  const ask = useCallback(
    async (raw: string): Promise<boolean> => {
      const text = raw.trim().slice(0, MAX_INPUT);
      if (!text) return true;
      if (inFlight.current) {
        queued.current = text;
        return true;
      }
      const mine = ++generation.current;
      const controller = new AbortController();
      inFlight.current = controller;
      const question: Turn = { role: "user", content: text };
      const asked = [...current.current, question];
      const from = here();
      commit(asked);
      setProblem(null);
      setSending(true);
      setStep(null);
      let answer: AssistantAnswer | null = null;
      try {
        answer = await sendToAssistant(toMessages(asked), pageForAssistant(window.location.pathname, window.location.search), {
          signal: controller.signal,
          conversation: chatId.current,
          locale,
          onStep: (s) => {
            if (generation.current === mine) setStep(s);
          },
          onCart: hydrate,
        });
      } catch {
        answer = null;
      }
      if (generation.current !== mine) return true;
      inFlight.current = null;
      setSending(false);
      setStep(null);
      if (!answer?.ok) {
        queued.current = null;
        if (current.current.at(-1) === question) commit(current.current.slice(0, -1));
        releaseHeld();
        setProblem({ kind: answer?.problem ?? "failed", text });
        return false;
      }
      if (answer.cart !== undefined) hydrate(answer.cart);
      if (answer.navigate && isStorePath(answer.navigate.path) && here() === from) router.push(assistantHref(answer.navigate), { scroll: false });
      setArrived(current.current.length);
      commit([
        ...current.current,
        {
          role: "assistant",
          content: answer.reply,
          recap: answer.recap,
          followUps: answer.followUps,
          detail: answer.detail,
          links: answer.links,
          navigated: answer.navigate,
          products: answer.products,
          showCart: answer.checkout || answer.cartChanged,
          incomplete: answer.incomplete,
          ...(answer.cartLost && { cartLost: true }),
          ...(answer.cartMissed?.length && { cartMissed: answer.cartMissed }),
        },
      ]);
      releaseHeld();
      const next = queued.current;
      queued.current = null;
      if (next) void askRef.current(next);
      return true;
    },
    [commit, hydrate, router, locale, releaseHeld],
  );

  useEffect(() => {
    askRef.current = ask;
  }, [ask]);

  // One the shopper brings in while a turn is out waits for its answer, so the chat keeps its order
  const seed = useCallback(
    (exchange: Exchange) => {
      if (inFlight.current) held.current = exchange;
      else carryOn(exchange);
    },
    [carryOn],
  );

  const newChat = useCallback(() => {
    generation.current += 1;
    inFlight.current?.abort();
    inFlight.current = null;
    queued.current = null;
    held.current = null;
    setSending(false);
    setStep(null);
    setProblem(null);
    setArrived(-1);
    chatId.current = newChatId();
    commit([]);
  }, [commit]);

  return { turns, sending, step, problem, arrived, ask, seed, newChat };
}
