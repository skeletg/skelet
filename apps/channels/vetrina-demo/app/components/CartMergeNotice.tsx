"use client";

// Post-login strip for the one case the customer must not discover on their own: the guest cart couldn't be merged. The lines are still on
// the guest cart cookie, so this says "not lost, try again" rather than leaving an empty basket unexplained

import { useEffect, useState } from "react";

import { CART_MERGE_FAILED, CART_MERGE_FLAG } from "@/app/lib/cart/merge-flag";

export function CartMergeNotice() {
  const [show, setShow] = useState(false);

  // The flag lives in the URL and has to be read after hydration, then stripped - reading
  // window during render would desync the server pass
  useEffect(() => {
    const url = new URL(window.location.href);
    if (url.searchParams.get(CART_MERGE_FLAG) !== CART_MERGE_FAILED) return;
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setShow(true);
    // Strip it so a refresh or a shared link doesn't replay the warning
    url.searchParams.delete(CART_MERGE_FLAG);
    window.history.replaceState(null, "", url.pathname + url.search + url.hash);
  }, []);

  if (!show) return null;

  return (
    <div
      role="status"
      className="flex flex-wrap items-center justify-center gap-3 bg-amber-500 px-4 py-1.5 text-center text-sm font-medium text-black"
    >
      <span>
        We couldn&rsquo;t move your basket over just now - nothing is lost.
        Reload the cart in a moment and it should be there.
      </span>
      <button
        type="button"
        onClick={() => setShow(false)}
        className="underline underline-offset-2"
      >
        Dismiss
      </button>
    </div>
  );
}
