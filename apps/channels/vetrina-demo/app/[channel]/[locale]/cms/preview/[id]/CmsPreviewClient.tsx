"use client";

// Live CMS preview iframe - takes the server-rendered tree, then re-renders from the dashboard's postMessage edits. Contract + the
// static/dynamic split: ARCHITECTURE.md Messages whose pageId doesn't match are dropped, so two open previews can't bleed

import { useEffect, useRef, useState, type ReactNode } from "react";

import type { Block } from "@/app/components/cms/blocks";
import { readString } from "@/app/components/cms/blocks";
import { HeroBlock } from "@/app/components/cms/HeroBlock";
import { CtaBannerBlock } from "@/app/components/cms/CtaBannerBlock";
import { HtmlBlock } from "@/app/components/cms/HtmlBlock";
import { CmsSection } from "@/app/components/cms/Section";
import { renderPreviewBlock } from "./actions";
import { DASHBOARD_ORIGIN } from "@/app/lib/dashboard-origin";

const MESSAGE_TYPE = "skelet:cms-preview:update";
const READY_TYPE = "skelet:cms-preview:ready";
const SELECT_TYPE = "skelet:cms-preview:select";

const STATIC_RENDERERS: Record<string, (props: { block: Block }) => ReactNode> =
  {
    hero: HeroBlock,
    cta_banner: CtaBannerBlock,
    html: HtmlBlock,
  };

type Props = {
  pageId: string;
  token: string;
  initialBlocks: Block[] | null;
  initialServerRender: ReactNode;
};

export function CmsPreviewClient({
  pageId,
  token,
  initialBlocks,
  initialServerRender,
}: Props) {
  const [blocks, setBlocks] = useState<Block[] | null>(null);
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    function onMessage(ev: MessageEvent) {
      if (ev.origin !== DASHBOARD_ORIGIN) return;
      const data = ev.data;
      if (!data || typeof data !== "object") return;
      if (data.type !== MESSAGE_TYPE) return;
      if (data.pageId !== pageId) return;
      const incoming = data.blocks;
      if (!Array.isArray(incoming)) return;
      if (process.env.NODE_ENV !== "production") {
        console.debug("[cms-preview] receive", {
          pageId,
          blockCount: incoming.length,
          dirty: !!data.dirty,
        });
      }
      setBlocks(incoming as Block[]);
      setDirty(!!data.dirty);
    }
    window.addEventListener("message", onMessage);
    // Tell the parent we're alive. Sent immediately and after short delays so a render-ordering race can't drop the
    // handshake (parent listener installed AFTER iframe load → first :ready missed; the retries catch that)
    function announce() {
      window.parent?.postMessage({ type: READY_TYPE, pageId }, DASHBOARD_ORIGIN);
    }
    announce();
    const t1 = window.setTimeout(announce, 100);
    const t2 = window.setTimeout(announce, 500);
    return () => {
      window.removeEventListener("message", onMessage);
      window.clearTimeout(t1);
      window.clearTimeout(t2);
    };
  }, [pageId]);

  // When the doc returns to the saved state (a save, or undo back to baseline), reload so the server render reflects
  // the now-current saved content - picks up freshly-saved grid data instead of the stale initial paint
  const prevDirtyRef = useRef(false);
  useEffect(() => {
    const wasDirty = prevDirtyRef.current;
    prevDirtyRef.current = dirty;
    if (wasDirty && !dirty) window.location.reload();
  }, [dirty]);

  function onSelectClick(e: React.MouseEvent<HTMLElement>) {
    const el = (e.target as HTMLElement).closest("[data-block-id]");
    if (!el) return;
    e.preventDefault();
    const blockId = el.getAttribute("data-block-id");
    if (blockId) {
      window.parent?.postMessage({ type: SELECT_TYPE, pageId, blockId }, DASHBOARD_ORIGIN);
    }
  }

  const hasInitial = (initialBlocks?.length ?? 0) > 0;

  return (
    <main className="flex flex-col" onClickCapture={onSelectClick}>
      <Banner dirty={dirty} />
      {!dirty ? (
        hasInitial ? (
          initialServerRender
        ) : (
          <EmptyState />
        )
      ) : blocks && blocks.length > 0 ? (
        <ClientBlocks blocks={blocks} pageId={pageId} token={token} />
      ) : (
        <EmptyState />
      )}
    </main>
  );
}
function ClientBlocks({
  blocks,
  pageId,
  token,
}: {
  blocks: Block[];
  pageId: string;
  token: string;
}) {
  return (
    <>
      {blocks.map((block) => {
        const Renderer = STATIC_RENDERERS[block.type];
        return (
          <SelectableBlock key={block.id} blockId={block.id}>
            {Renderer ? (
              <Renderer block={block} />
            ) : (
              <ServerDynamicBlock block={block} pageId={pageId} token={token} />
            )}
          </SelectableBlock>
        );
      })}
    </>
  );
}

function SelectableBlock({ blockId, children }: { blockId: string; children: ReactNode }) {
  return (
    <div
      data-block-id={blockId}
      className="group/cms relative cursor-pointer outline-offset-[-2px] transition-[outline] hover:outline hover:outline-2 hover:outline-sky-400"
    >
      <span className="pointer-events-none absolute right-2 top-2 z-10 hidden rounded bg-sky-500 px-1.5 py-0.5 text-[10px] font-medium text-white shadow group-hover/cms:inline-block">
        Click to edit
      </span>
      {children}
    </div>
  );
}

// Server-renders a non-static block (grids, accordion, tabs, ...) to its real form via a
// debounced server action, so the live preview shows the ACTUAL component with real data -
// a brief loading state shows until the first render lands, the previous render stays on
// screen while a re-render is in flight, and a request counter drops stale out-of-order responses
// A failure surfaces as an inline error with a retry, instead of loading forever
function ServerDynamicBlock({
  block,
  pageId,
  token,
}: {
  block: Block;
  pageId: string;
  token: string;
}) {
  const [node, setNode] = useState<ReactNode>(null);
  const [error, setError] = useState<string | null>(null);
  const [attempt, setAttempt] = useState(0);
  const latest = useRef(0);
  const key = JSON.stringify(block);
  useEffect(() => {
    const id = ++latest.current;
    const t = window.setTimeout(() => {
      renderPreviewBlock(pageId, token, block)
        .then((res) => {
          if (id !== latest.current) return;
          if (res.ok) {
            setNode(res.node);
            setError(null);
          } else {
            setError(res.error);
          }
        })
        .catch(() => {
          if (id === latest.current) setError("The server-side render failed");
        });
    }, 250);
    return () => window.clearTimeout(t);
    // key captures every prop change - block is stable within a given key
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, attempt, pageId, token]);

  if (error !== null) {
    return (
      <DynamicBlockError
        block={block}
        message={error}
        onRetry={() => {
          setError(null);
          setAttempt((a) => a + 1);
        }}
      />
    );
  }
  if (node !== null) return <>{node}</>;
  return <DynamicBlockLoading block={block} />;
}

function DynamicBlockError({
  block,
  message,
  onRetry,
}: {
  block: Block;
  message: string;
  onRetry: () => void;
}) {
  const heading = readString(block.props, "heading");
  const label = DYNAMIC_LABELS[block.type] ?? block.type;
  return (
    <CmsSection heading={heading}>
      <div className="rounded-2xl border border-dashed border-red-300 bg-red-50 px-6 py-10 text-center text-sm text-red-700">
        <p>
          Couldn&apos;t render {label.toLowerCase()}: {message}
        </p>
        <button
          type="button"
          onClick={onRetry}
          className="mt-3 rounded border border-red-300 bg-white px-3 py-1 text-xs font-medium text-red-700 transition hover:bg-red-100"
        >
          Retry
        </button>
      </div>
    </CmsSection>
  );
}

// The transient state shown until the server render lands
function DynamicBlockLoading({ block }: { block: Block }) {
  const heading = readString(block.props, "heading");
  const label = DYNAMIC_LABELS[block.type] ?? block.type;
  return (
    <CmsSection heading={heading}>
      <div className="rounded-2xl border border-dashed border-border bg-muted/30 px-6 py-10 text-center text-sm text-muted-foreground">
        Rendering {label.toLowerCase()} with live data…
      </div>
    </CmsSection>
  );
}

const DYNAMIC_LABELS: Record<string, string> = {
  product_grid: "Product grid",
  collection_grid: "Collection grid",
  category_grid: "Category grid",
  blog_grid: "Blog grid",
};

function EmptyState() {
  return (
    <div className="mx-auto w-full max-w-3xl px-6 py-16 text-center text-sm text-muted-foreground">
      No blocks yet. Add a block in the dashboard to see it here.
    </div>
  );
}

function Banner({ dirty }: { dirty: boolean }) {
  return (
    <div className="border-b border-amber-500 bg-amber-50 px-4 py-2 text-center text-xs text-amber-900">
      {dirty
        ? "Preview mode - showing unsaved edits, rendered with live data."
        : "Preview mode - showing the saved page. Edit in the dashboard to see changes stream in."}
    </div>
  );
}
