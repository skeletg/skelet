"use client";

// Live blog preview iframe - server-fetched initial state, then re-renders from the dashboard's postMessage edits.
// Contract: ARCHITECTURE.md Messages whose postId doesn't match are dropped, so two open previews can't bleed

import { useEffect, useState } from "react";

import { displayName } from "@skelet/storefront-core/display";
import { formatDate } from "@skelet/storefront-core/format";
import { LexicalRichText } from "@/app/lib/lexical-render";
import { lexicalHasContent, lexicalPlainText } from "@skelet/storefront-core/lexical";
import { DASHBOARD_ORIGIN } from "@skelet/storefront-core/dashboard-origin";

type PreviewPost = {
  id: string;
  slug: string;
  publishedAt: string | null;
  readingTimeMinutes: number | null;
  seoNoindex: boolean;
  author: { id: string; slug: string; displayName: string } | null;
  content: {
    locale: string;
    title: string;
    excerpt: unknown;
    body: unknown;
    metaTitle: string | null;
    metaDescription: string | null;
  } | null;
  categories: {
    id: string;
    slug: string;
    urlPath: string;
    content: { name: string } | null;
  }[];
  tags: {
    id: string;
    slug: string;
    content: { name: string } | null;
  }[];
};

type Props = {
  initialPost: PreviewPost | null;
  postId: string;
};

const MESSAGE_TYPE = "skelet:blog-preview:update";

export function PreviewClient({ initialPost, postId }: Props) {
  const [post, setPost] = useState<PreviewPost | null>(initialPost);

  useEffect(() => {
    function onMessage(ev: MessageEvent) {
      if (ev.origin !== DASHBOARD_ORIGIN) return;
      const data = ev.data;
      if (!data || typeof data !== "object") return;
      if (data.type !== MESSAGE_TYPE) return;
      if (data.postId !== postId) return;
      const incoming = data.post;
      if (!incoming || typeof incoming !== "object") return;
      if (process.env.NODE_ENV !== "production") {
        console.debug("[blog-preview] receive", {
          postId,
          contentTitle: (incoming as Partial<PreviewPost>).content?.title,
        });
      }
      // Merge so a partial update doesn't blank out fields the dashboard didn't touch
      // (e.g. categories/tags when the editor only changed the body)
      setPost((prev) => mergePost(prev, incoming as Partial<PreviewPost>));
    }
    window.addEventListener("message", onMessage);
    // Tell the parent we're alive. Sent both immediately and after a short delay so a stray render ordering can't drop
    // the handshake (parent listener installed AFTER iframe load → first :ready is missed; the retry catches that case)
    function announce() {
      window.parent?.postMessage(
        { type: "skelet:blog-preview:ready", postId },
        DASHBOARD_ORIGIN,
      );
    }
    announce();
    const t1 = window.setTimeout(announce, 100);
    const t2 = window.setTimeout(announce, 500);
    return () => {
      window.removeEventListener("message", onMessage);
      window.clearTimeout(t1);
      window.clearTimeout(t2);
    };
  }, [postId]);

  if (!post) {
    return (
      <main className="mx-auto w-full max-w-3xl px-6 pt-6 pb-12">
        <Banner />
        <p className="mt-8 text-sm text-muted-foreground">
          Preview session expired or unavailable. Close and reopen
          preview from the dashboard.
        </p>
      </main>
    );
  }

  const title = displayName(post.content?.title, post.slug);
  const excerpt = lexicalPlainText(post.content?.excerpt) || null;
  const hasBody = lexicalHasContent(post.content?.body);

  return (
    <main className="mx-auto w-full max-w-3xl px-6 pt-6 pb-12">
      <Banner />
      <article className="mt-6">
        <header className="border-b border-border pb-6">
          <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1 text-xs text-muted-foreground">
            {post.publishedAt && (
              <time dateTime={post.publishedAt}>
                {formatDate(post.publishedAt)}
              </time>
            )}
            {post.author && (
              <>
                <span>·</span>
                <span>{post.author.displayName}</span>
              </>
            )}
            {post.readingTimeMinutes && (
              <>
                <span>·</span>
                <span>{post.readingTimeMinutes} min read</span>
              </>
            )}
          </div>
          <h1 className="mt-2 text-3xl font-extrabold tracking-tight">
            {title}
          </h1>
          {excerpt && (
            <p className="mt-3 text-base leading-relaxed text-muted-foreground">
              {excerpt}
            </p>
          )}
        </header>

        {hasBody ? (
          <div className="mt-8 space-y-4 text-base leading-relaxed">
            <LexicalRichText value={post.content?.body} />
          </div>
        ) : (
          <p className="mt-8 text-sm text-muted-foreground">
            No body yet.
          </p>
        )}
      </article>
    </main>
  );
}

function Banner() {
  return (
    <div className="rounded border border-amber-500 bg-amber-50 px-3 py-2 text-xs text-amber-900">
      Preview mode - not visible to public. Edits in the dashboard
      stream into this view live.
    </div>
  );
}
function mergePost(
  prev: PreviewPost | null,
  patch: Partial<PreviewPost>,
): PreviewPost {
  const base = prev ?? {
    id: "",
    slug: "",
    publishedAt: null,
    readingTimeMinutes: null,
    seoNoindex: false,
    author: null,
    content: null,
    categories: [],
    tags: [],
  };
  return { ...base, ...patch };
}
