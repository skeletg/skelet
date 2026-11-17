"use client";

import { Component, Suspense, type ErrorInfo, type ReactNode } from "react";
import { getSlots } from "./registry";

// A slot that throws stays its own failure: the page around it - the checkout, or every page for one in the header -
// carries on without it
class SlotBoundary extends Component<{ id: string; children: ReactNode }, { failed: boolean }> {
  state = { failed: false };

  static getDerivedStateFromError() {
    return { failed: true };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error(`storefront plugin slot "${this.props.id}" failed to render`, error, info.componentStack);
  }

  render() {
    return this.state.failed ? null : this.props.children;
  }
}

// A slot's render runs inside the boundary only as a component of its own - called inline it would throw in the
// page's render, above the boundary
function SlotEntry<TCtx>({ render, ctx }: { render: (ctx: TCtx) => ReactNode; ctx: TCtx }) {
  return <>{render(ctx)}</>;
}

// Renders whatever is registered for one zone, nothing at all when no plugin claims it - ctx is handed to each slot's
// render. The Suspense is for the server, where no boundary catches: a slot failing there is left out of the HTML and
// tried again in the browser, where the boundary has it
export function Slots<TCtx = unknown>({ zone, ctx }: { zone: string; ctx?: TCtx }) {
  const entries = getSlots<TCtx>(zone);
  if (entries.length === 0) return null;
  return (
    <>
      {entries.map((s) => (
        <span key={`${s.zone}:${s.id}`} className="contents">
          <Suspense fallback={null}>
            <SlotBoundary id={`${s.zone}:${s.id}`}>
              <SlotEntry render={s.render} ctx={ctx as TCtx} />
            </SlotBoundary>
          </Suspense>
        </span>
      ))}
    </>
  );
}
