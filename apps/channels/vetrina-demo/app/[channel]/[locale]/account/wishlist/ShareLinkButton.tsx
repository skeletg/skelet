"use client";

import { useState } from "react";

export function ShareLinkButton({ token }: { token: string }) {
  const [copied, setCopied] = useState(false);

  return (
    <button
      type="button"
      onClick={() => {
        const url = `${window.location.origin}/wishlist/${token}`;
        void navigator.clipboard.writeText(url).then(() => {
          setCopied(true);
          setTimeout(() => setCopied(false), 2000);
        });
      }}
      className="inline-flex h-10 items-center rounded-md border border-border bg-background px-5 text-sm font-semibold transition-colors hover:bg-accent"
    >
      {copied ? "Link copied!" : "Copy share link"}
    </button>
  );
}
