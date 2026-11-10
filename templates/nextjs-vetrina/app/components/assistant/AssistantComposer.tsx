"use client";

import { useEffect, useImperativeHandle, useLayoutEffect, useRef, useState, type KeyboardEvent, type Ref } from "react";
import { ArrowUpIcon, LoaderCircleIcon } from "lucide-react";
import { useTranslations } from "next-intl";

import { MAX_INPUT } from "./useAssistantChat";

const MAX_INPUT_HEIGHT = 128;

export type ComposerHandle = { retry: (text: string) => void };

type Props = { ref?: Ref<ComposerHandle>; onSend: (text: string) => Promise<boolean>; sending: boolean; focus: boolean };

// Keeps what's typed to itself, so typing never redraws the conversation - and a failed send gets its words back
export function AssistantComposer({ ref, onSend, sending, focus }: Props) {
  const t = useTranslations("assistant");
  const [input, setInput] = useState("");
  const inputRef = useRef<HTMLTextAreaElement>(null);

  useLayoutEffect(() => {
    const el = inputRef.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight, MAX_INPUT_HEIGHT)}px`;
    el.style.overflowY = el.scrollHeight > MAX_INPUT_HEIGHT ? "auto" : "hidden";
  }, [input]);

  useEffect(() => {
    if (focus) inputRef.current?.focus();
  }, [focus]);

  async function send(text: string) {
    if (sending || text.trim() === "") return;
    setInput((now) => (now === text ? "" : now));
    if (!(await onSend(text))) setInput((now) => (now === "" ? text : now));
  }

  useImperativeHandle(ref, () => ({ retry: (text) => void send(text) }));

  function onKey(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault();
      void send(input);
    }
  }

  return (
    <form
      className="px-3 pb-3 pt-1"
      onSubmit={(e) => {
        e.preventDefault();
        void send(input);
      }}
    >
      <div className="assistant-compose relative isolate" data-busy={sending ? "" : undefined}>
        <div aria-hidden="true" className="assistant-aura">
          <div className="assistant-spin" />
        </div>
        <div
          onClick={() => inputRef.current?.focus()}
          className="assistant-field relative flex cursor-text items-end gap-2 rounded-2xl border border-border bg-background py-2 pl-4 pr-2 shadow-[0_1px_2px_oklch(0_0_0/0.04),0_6px_20px_-12px_oklch(0_0_0/0.18)] transition-shadow focus-within:shadow-[0_1px_2px_oklch(0_0_0/0.05),0_10px_28px_-14px_oklch(0_0_0/0.28)]"
        >
          <span aria-hidden="true" className="assistant-ring assistant-field__ring">
            <span className="assistant-spin" />
          </span>
          <textarea
            ref={inputRef}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={onKey}
            rows={1}
            maxLength={MAX_INPUT}
            placeholder={t("placeholder")}
            aria-label={t("placeholder")}
            className="block min-h-8 flex-1 resize-none overflow-y-hidden bg-transparent py-1 text-sm leading-6 outline-none placeholder:text-muted-foreground"
          />
          <button
            type="submit"
            disabled={sending || input.trim() === ""}
            aria-label={t("send")}
            className="flex size-8 shrink-0 items-center justify-center rounded-full bg-foreground text-background transition-[background-color,color,scale] hover:bg-foreground/85 active:scale-95 disabled:bg-muted disabled:text-muted-foreground"
          >
            {sending ? (
              <LoaderCircleIcon className="size-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
            ) : (
              <ArrowUpIcon className="size-4" strokeWidth={2.25} aria-hidden="true" />
            )}
          </button>
        </div>
      </div>
      <p className="mt-2 text-center text-[11px] text-muted-foreground">{t("disclaimer")}</p>
    </form>
  );
}
