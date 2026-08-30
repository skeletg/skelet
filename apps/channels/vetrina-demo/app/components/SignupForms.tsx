"use client";

// Storefront signup-form widget: fetches the channel's active forms from the
// same-origin /t/forms feed and renders the first eligible one as a popup, flyout, or
// inline banner per its display rules (immediate / delay / scroll / exit-intent), with
// a per-form frequency cap enforced by a first-party cookie

import { useEffect, useState } from "react";

type Display = {
  trigger: string;
  delaySeconds: number;
  scrollPercent: number;
  frequencyDays: number;
};
type FormDef = {
  id: string;
  slug: string;
  type: string; // popup | inline | flyout
  heading: string;
  body: string;
  buttonLabel: string;
  successMessage: string;
  collectName: boolean;
  display: Display;
};

function readCookie(name: string): string {
  if (typeof document === "undefined") return "";
  const m = document.cookie.match(new RegExp("(?:^|; )" + name + "=([^;]*)"));
  return m ? decodeURIComponent(m[1]) : "";
}
function setCookie(name: string, value: string, days: number) {
  const maxAge = Math.max(1, days) * 24 * 60 * 60;
  document.cookie = `${name}=${encodeURIComponent(value)}; max-age=${maxAge}; path=/; samesite=lax`;
}
const seenCookie = (slug: string) => `signup_form_${slug}`;

export function SignupForms({ channelId }: { channelId: string }) {
  const [form, setForm] = useState<FormDef | null>(null);
  const [visible, setVisible] = useState(false);

  // Load the active forms once, pick the first not-recently-dismissed one
  useEffect(() => {
    let cancelled = false;
    fetch("/t/forms", { headers: { "X-Channel-Id": channelId } })
      .then((r) => (r.ok ? r.json() : []))
      .then((forms: FormDef[]) => {
        if (cancelled) return;
        const eligible = forms.find((f) => !readCookie(seenCookie(f.slug)));
        if (eligible) setForm(eligible);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [channelId]);

  // One trigger machine for all display modes - only the immediate branch lands in the same
  // tick, the rest arm a timer or a listener
  useEffect(() => {
    if (!form) return;
    const d = form.display;
    if (form.type === "inline" || d.trigger === "immediate") {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setVisible(true);
      return;
    }
    if (d.trigger === "delay") {
      const t = window.setTimeout(() => setVisible(true), Math.max(0, d.delaySeconds) * 1000);
      return () => window.clearTimeout(t);
    }
    if (d.trigger === "scroll") {
      const onScroll = () => {
        const pct =
          (window.scrollY / Math.max(1, document.body.scrollHeight - window.innerHeight)) * 100;
        if (pct >= (d.scrollPercent || 50)) setVisible(true);
      };
      window.addEventListener("scroll", onScroll, { passive: true });
      return () => window.removeEventListener("scroll", onScroll);
    }
    if (d.trigger === "exit_intent") {
      const onLeave = (e: MouseEvent) => {
        if (e.clientY <= 0) setVisible(true);
      };
      document.addEventListener("mouseout", onLeave);
      return () => document.removeEventListener("mouseout", onLeave);
    }
  }, [form]);

  if (!form || !visible) return null;

  const dismiss = () => {
    setCookie(seenCookie(form.slug), "1", form.display.frequencyDays || 30);
    setVisible(false);
  };

  return <FormCard form={form} channelId={channelId} onDismiss={dismiss} />;
}

function FormCard({
  form,
  channelId,
  onDismiss,
}: {
  form: FormDef;
  channelId: string;
  onDismiss: () => void;
}) {
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [state, setState] = useState<"idle" | "sending" | "done" | "error">("idle");
  const [msg, setMsg] = useState(form.successMessage);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!email.trim() || state === "sending") return;
    setState("sending");
    try {
      const res = await fetch("/t/forms/submit", {
        method: "POST",
        headers: { "Content-Type": "application/json", "X-Channel-Id": channelId },
        body: JSON.stringify({ form: form.slug, email: email.trim(), name: name.trim() }),
      });
      if (res.ok) {
        const body = (await res.json().catch(() => ({}))) as { message?: string };
        if (body.message) setMsg(body.message);
        setState("done");
        setCookie(seenCookie(form.slug), "1", 365);
      } else {
        setState("error");
      }
    } catch {
      setState("error");
    }
  };

  const inner = (
    <div className="w-full max-w-md rounded-2xl border border-border bg-card p-6 shadow-xl">
      <div className="flex items-start justify-between gap-4">
        <h2 className="text-lg font-semibold">{form.heading || "Join our list"}</h2>
        {form.type !== "inline" && (
          <button onClick={onDismiss} aria-label="Close" className="text-muted-foreground hover:text-foreground">
            ✕
          </button>
        )}
      </div>
      {form.body && <p className="mt-1 text-sm text-muted-foreground">{form.body}</p>}
      {state === "done" ? (
        <p className="mt-4 rounded-lg bg-emerald-50 p-3 text-sm text-emerald-800 dark:bg-emerald-950 dark:text-emerald-200">
          {msg}
        </p>
      ) : (
        <form onSubmit={submit} className="mt-4 grid gap-2">
          {form.collectName && (
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="First name"
              className="rounded-lg border border-input bg-card px-3 py-2 text-sm outline-none focus:border-ring"
            />
          )}
          <input
            type="email"
            required
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="you@example.com"
            className="rounded-lg border border-input bg-card px-3 py-2 text-sm outline-none focus:border-ring"
          />
          <button
            type="submit"
            disabled={state === "sending"}
            className="rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-60"
          >
            {state === "sending" ? "…" : form.buttonLabel || "Subscribe"}
          </button>
          {state === "error" && <p className="text-xs text-rose-600">Something went wrong - please try again.</p>}
        </form>
      )}
    </div>
  );

  if (form.type === "inline") {
    return <div className="mx-auto my-6 px-4">{inner}</div>;
  }
  if (form.type === "flyout") {
    return <div className="fixed bottom-4 right-4 z-50 max-w-sm">{inner}</div>;
  }
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4" onClick={onDismiss}>
      <div onClick={(e) => e.stopPropagation()}>{inner}</div>
    </div>
  );
}
