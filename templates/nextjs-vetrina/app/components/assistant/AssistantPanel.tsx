"use client";

import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties, type MouseEvent } from "react";
import { XIcon } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";

import { readAssistantPage } from "@skelet/storefront-core/assistant/client";
import { pageForAssistant } from "@skelet/storefront-core/assistant/page";
import type { AssistantPage } from "@skelet/storefront-core/assistant/types";
import { useCartUI } from "@/app/components/CartUIProvider";
import { useChannel } from "@/app/components/ChannelProvider";
import { Drawer } from "@/app/components/Drawer";
import { usePathname } from "@/app/components/LocaleLink";
import { usePhone } from "@/app/lib/use-phone";

import { AssistantComposer, type ComposerHandle } from "./AssistantComposer";
import { PANEL_ID, useAssistant } from "./AssistantProvider";
import { AssistantThread } from "./AssistantThread";
import { MAX_W, MIN_W, ResizeHandle, panelInset } from "./ResizeHandle";
import { useAssistantChat } from "./useAssistantChat";

const DEFAULT_W = 520;
const WIDTH_KEY = "assistant:panel-width";
const GLOW_FADE_MS = 450;

function savedWidth(): number {
  try {
    const w = Number(localStorage.getItem(WIDTH_KEY));
    return w >= MIN_W && w <= MAX_W ? w : DEFAULT_W;
  } catch {
    return DEFAULT_W;
  }
}

function originsOf(publicUrl: string | undefined): string[] {
  if (typeof window === "undefined") return [];
  const out = [window.location.origin];
  try {
    const store = publicUrl ? new URL(publicUrl).origin : "null";
    if (store !== "null" && !out.includes(store)) out.push(store);
  } catch {
    // a malformed public URL just isn't one of the store's links
  }
  return out;
}

function EdgeGlow({ busy }: { busy: boolean }) {
  return (
    <div data-busy={busy ? "" : undefined} className="assistant-glow">
      <div className="assistant-glow__light">
        <div className="assistant-ring assistant-ring--soft">
          <div className="assistant-spin assistant-spin--page" />
        </div>
      </div>
      <div className="assistant-ring assistant-glow__line">
        <div className="assistant-spin assistant-spin--page" />
      </div>
    </div>
  );
}

function useGlowing(open: boolean): boolean {
  const [glowing, setGlowing] = useState(open);
  if (open && !glowing) setGlowing(true);
  useEffect(() => {
    if (open) return;
    const t = setTimeout(() => setGlowing(false), GLOW_FADE_MS);
    return () => clearTimeout(t);
  }, [open]);
  return glowing;
}

// A full-height layout column that pushes the page narrower on wide screens, and the site's own drawer on a phone
export function AssistantPanel({ storeName }: { storeName: string }) {
  const { name, open, close, onAsk, onContinue, trigger } = useAssistant();
  const phone = usePhone();
  const pathname = usePathname();
  const locale = useLocale();
  const t = useTranslations("assistant");
  const channel = useChannel();
  const { open: openCart } = useCartUI();
  const { turns, sending, step, problem, arrived, ask, seed, newChat } = useAssistantChat();
  const [pageInfo, setPageInfo] = useState<AssistantPage | null>(null);
  const [width, setWidth] = useState(DEFAULT_W);
  const [resizing, setResizing] = useState(false);
  const [chatId, setChatId] = useState(0);
  const [entered, setEntered] = useState(false);
  const panelRef = useRef<HTMLElement>(null);
  const composer = useRef<ComposerHandle>(null);
  const storeOrigins = useMemo(() => originsOf(channel?.publicUrl), [channel?.publicUrl]);
  const shown = open && entered;
  const glowing = useGlowing(shown);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setWidth(savedWidth());
    const frame = requestAnimationFrame(() => setEntered(true)); // one frame shut, so the first open slides in too
    return () => cancelAnimationFrame(frame);
  }, []);

  useEffect(() => {
    if (!shown || phone) return;
    const root = document.documentElement;
    root.style.setProperty("--assistant-inset", panelInset(width));
    root.dataset.assistantOpen = "";
    return () => {
      root.style.removeProperty("--assistant-inset");
      delete root.dataset.assistantOpen;
    };
  }, [shown, phone, width]);

  useEffect(() => {
    onAsk((text) => void ask(text));
    return () => onAsk(null);
  }, [onAsk, ask]);

  useEffect(() => {
    onContinue(seed);
    return () => onContinue(null);
  }, [onContinue, seed]);

  const empty = turns !== null && turns.length === 0;
  useEffect(() => {
    if (!open || !empty) return;
    const page = pageForAssistant(window.location.pathname, window.location.search);
    const controller = new AbortController();
    void (page ? readAssistantPage(page, { signal: controller.signal, locale }) : Promise.resolve(null)).then((info) => {
      if (!controller.signal.aborted) setPageInfo(info);
    });
    return () => controller.abort();
  }, [open, empty, pathname, locale]);

  const closePanel = useCallback(() => {
    if (panelRef.current?.contains(document.activeElement)) trigger.current?.focus();
    close();
  }, [close, trigger]);

  const startOver = useCallback(() => {
    newChat();
    setChatId((n) => n + 1);
  }, [newChat]);

  const send = useCallback((text: string) => void ask(text), [ask]);

  const retry = useCallback(() => {
    if (problem) composer.current?.retry(problem.text);
  }, [problem]);

  const commitWidth = useCallback((w: number) => {
    setWidth(w);
    try {
      localStorage.setItem(WIDTH_KEY, String(w));
    } catch {
      // the width just won't be remembered
    }
  }, []);

  function closeOnLink(e: MouseEvent) {
    if ((e.target as Element).closest("a[href]")) close();
  }

  const chat = (
    <>
      <div className={`flex items-center gap-2 border-b border-border px-4 py-3 ${phone ? "pr-12" : ""}`}>
        <span className="text-base font-semibold">{name || t("title")}</span>
        <button type="button" onClick={startOver} className="ml-auto rounded-md px-2 py-1 text-xs text-muted-foreground transition hover:bg-muted hover:text-foreground">
          {t("newChat")}
        </button>
        {phone ? null : (
          <button type="button" onClick={closePanel} aria-label={t("close")} className="flex h-8 w-8 items-center justify-center rounded-md transition hover:bg-muted">
            <XIcon className="size-4" strokeWidth={1.8} aria-hidden="true" />
          </button>
        )}
      </div>
      <AssistantThread
        turns={turns ?? []}
        sending={sending}
        step={step}
        problem={problem}
        arrived={arrived}
        storeName={storeName}
        pageInfo={pageInfo}
        storeOrigins={storeOrigins}
        open={shown}
        onAsk={send}
        onRetry={retry}
        onOpenCart={openCart}
        onSeeList={phone ? close : undefined}
      />
      <AssistantComposer key={chatId} ref={composer} onSend={ask} sending={sending} focus={shown && !phone} />
    </>
  );
  if (phone) {
    return (
      <Drawer open={shown} onOpenChange={(next) => (next ? undefined : close())} side="right" title={name || t("title")} hideTitle className="h-[90dvh]!">
        <div id={PANEL_ID} className="flex min-h-0 flex-1 flex-col" onClickCapture={closeOnLink}>
          {chat}
        </div>
      </Drawer>
    );
  }
  return (
    <>
      <div aria-hidden="true" className={`assistant-frame ${resizing ? "" : "assistant-frame-ease"}`}>
        {glowing ? <EdgeGlow busy={sending} /> : null}
      </div>
      <aside
        id={PANEL_ID}
        ref={panelRef}
        aria-hidden={!shown}
        aria-label={name || t("title")}
        inert={!shown}
        onKeyDown={(e) => {
          if (e.key === "Escape") {
            e.stopPropagation();
            closePanel();
          }
        }}
        style={{ "--assistant-w": `${width}px` } as CSSProperties}
        className={`dark assistant-dark sticky top-0 z-[35] hidden h-dvh shrink-0 self-start justify-end overflow-hidden bg-(--assistant-gutter) text-foreground md:flex ${
          resizing ? "" : "transition-[width] duration-(--assistant-speed) ease-(--assistant-ease) motion-reduce:transition-none"
        } ${shown ? "w-[min(var(--assistant-w),var(--assistant-max-share))]" : "pointer-events-none w-0"}`}
      >
        <div
          className={`relative m-2 flex h-[calc(100%-1rem)] w-[calc(min(var(--assistant-w),var(--assistant-max-share))-1rem)] shrink-0 flex-col overflow-hidden rounded-xl bg-background shadow-xl transition-[translate,opacity] duration-(--assistant-speed) ease-(--assistant-ease) motion-reduce:transition-none ${
            shown ? "translate-x-0 opacity-100" : "translate-x-6 opacity-0"
          }`}
        >
          <ResizeHandle width={width} resizing={resizing} panel={panelRef} onResizing={setResizing} onCommit={commitWidth} />
          {chat}
        </div>
      </aside>
    </>
  );
}
