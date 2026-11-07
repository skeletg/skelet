"use client";

import { useRef, type KeyboardEvent, type PointerEvent, type RefObject } from "react";
import { useTranslations } from "next-intl";

export const MIN_W = 320;
export const MAX_W = 720;

const clampWidth = (w: number) => Math.min(MAX_W, Math.max(MIN_W, w));

export function panelInset(width: number): string {
  return `min(${width}px, var(--assistant-max-share))`;
}

type Props = { width: number; resizing: boolean; panel: RefObject<HTMLElement | null>; onResizing: (on: boolean) => void; onCommit: (width: number) => void };

// The panel's left edge. A drag writes the width straight onto the panel and the page's inset, and only the width it
// ends on goes through React - so a drag redraws nothing, and a cancelled touch still lets go
export function ResizeHandle({ width, resizing, panel, onResizing, onCommit }: Props) {
  const t = useTranslations("assistant");
  const drag = useRef<{ startX: number; startW: number; last: number } | null>(null);

  function show(w: number) {
    panel.current?.style.setProperty("--assistant-w", `${w}px`);
    document.documentElement.style.setProperty("--assistant-inset", panelInset(w));
  }

  function start(e: PointerEvent<HTMLDivElement>) {
    e.preventDefault();
    e.currentTarget.setPointerCapture(e.pointerId);
    drag.current = { startX: e.clientX, startW: width, last: width };
    document.documentElement.dataset.assistantResizing = "";
    document.body.style.userSelect = "none";
    onResizing(true);
  }

  function move(e: PointerEvent<HTMLDivElement>) {
    const d = drag.current;
    if (!d) return;
    d.last = clampWidth(d.startW + (d.startX - e.clientX));
    show(d.last);
  }

  function end() {
    const d = drag.current;
    if (!d) return;
    drag.current = null;
    delete document.documentElement.dataset.assistantResizing;
    document.body.style.userSelect = "";
    onResizing(false);
    onCommit(d.last);
  }

  function nudge(e: KeyboardEvent) {
    const step = e.key === "ArrowLeft" ? 16 : e.key === "ArrowRight" ? -16 : 0;
    if (step !== 0) onCommit(clampWidth(width + step));
  }

  return (
    <div
      role="separator"
      aria-orientation="vertical"
      aria-label={t("resize")}
      aria-valuenow={width}
      aria-valuemin={MIN_W}
      aria-valuemax={MAX_W}
      tabIndex={0}
      onPointerDown={start}
      onPointerMove={move}
      onPointerUp={end}
      onPointerCancel={end}
      onLostPointerCapture={end}
      onKeyDown={nudge}
      data-resizing={resizing ? "" : undefined}
      className="absolute inset-y-0 left-0 z-10 w-1.5 cursor-col-resize touch-none outline-none transition-colors hover:bg-signal/30 focus-visible:bg-signal/40 data-[resizing]:bg-signal/40"
    />
  );
}
