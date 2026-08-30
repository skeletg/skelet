"use client";

import { useEffect, useRef, type ReactNode } from "react";

export function TrackView({ event, children }: { event: string; children: ReactNode }) {
  const ref = useRef<HTMLDivElement>(null);
  const fired = useRef(false);

  useEffect(() => {
    const el = ref.current;
    if (!el || !event) return;
    const obs = new IntersectionObserver(
      (entries) => {
        for (const e of entries) {
          if (e.isIntersecting && !fired.current) {
            fired.current = true;
            (window.__lensQueue ??= []).push({ name: event, props: { source: "cms_block" } });
            window.lens?.flushQueue?.();
            obs.disconnect();
          }
        }
      },
      { threshold: 0.3 },
    );
    obs.observe(el);
    return () => obs.disconnect();
  }, [event]);

  return <div ref={ref}>{children}</div>;
}
