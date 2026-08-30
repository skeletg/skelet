"use client";

import { useState } from "react";

import { useCustomerIdentity } from "@/app/lib/customer/client";

function readCookie(name: string): string {
  if (typeof document === "undefined") return "";
  const m = document.cookie.match(new RegExp("(?:^|; )" + name + "=([^;]*)"));
  return m ? decodeURIComponent(m[1]) : "";
}

export function BackInStockForm({
  productId,
  sku,
  name,
  image,
  defaultEmail = "",
}: {
  productId: string;
  sku: string;
  name: string;
  image: string;
  defaultEmail?: string;
}) {
  const [state, setState] = useState<"idle" | "sending" | "done" | "error">("idle");
  // The page is cached HTML so the server render has no session - identity lands after
  // hydration and prefills the field, until the visitor types something of their own
  const { identity } = useCustomerIdentity();
  const [typed, setTyped] = useState<string | null>(null);
  const email = typed ?? identity?.email ?? defaultEmail;

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!email.trim() || state === "sending") return;
    setState("sending");
    try {
      // Channel id is stamped on <html data-channel-id> by the layout (the /t proxy
      // can't inject it), same source the Lens tracker reads
      const channelId = document.documentElement.dataset.channelId ?? "";
      const headers: Record<string, string> = {
        "Content-Type": "application/json",
        "X-Channel-Id": channelId,
      };
      const aid = readCookie("lens_aid");
      if (aid) headers["X-Anon-Id"] = aid;
      const res = await fetch("/t/back-in-stock", {
        method: "POST",
        headers,
        body: JSON.stringify({
          productId,
          sku,
          email: email.trim(),
          name,
          image,
          url: window.location.pathname,
        }),
      });
      setState(res.ok ? "done" : "error");
    } catch {
      setState("error");
    }
  };

  if (state === "done") {
    return (
      <div className="mt-6 rounded-xl border border-emerald-200 bg-emerald-50 p-4 text-sm text-emerald-800 dark:border-emerald-900 dark:bg-emerald-950 dark:text-emerald-200">
        You&apos;re on the list - we&apos;ll email you the moment it&apos;s back.
      </div>
    );
  }

  return (
    <form onSubmit={submit} className="mt-6 rounded-xl border border-border p-4">
      <p className="text-sm font-medium">Out of stock - get notified when it returns</p>
      <div className="mt-3 flex gap-2">
        <input
          type="email"
          required
          value={email}
          onChange={(e) => setTyped(e.target.value)}
          placeholder="you@example.com"
          className="min-w-0 flex-1 rounded-lg border border-border bg-background px-3 py-2 text-sm outline-none focus:border-ring"
        />
        <button
          type="submit"
          disabled={state === "sending"}
          className="shrink-0 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-60"
        >
          {state === "sending" ? "…" : "Notify me"}
        </button>
      </div>
      {state === "error" && (
        <p className="mt-2 text-xs text-rose-600">Something went wrong - please try again.</p>
      )}
    </form>
  );
}
