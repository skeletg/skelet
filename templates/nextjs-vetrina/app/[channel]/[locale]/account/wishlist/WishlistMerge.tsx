"use client";

import { useEffect, useRef } from "react";
import { useRouter } from "next/navigation";

import { readLocalWishlist } from "@/app/components/WishlistButton";
import { toggleWishlistAction } from "@skelet/storefront-next/wishlist/actions";

export function WishlistMerge({ serverIds }: { serverIds: string[] }) {
  const ran = useRef(false);
  const router = useRouter();
  useEffect(() => {
    if (ran.current) return;
    ran.current = true;
    const missing = readLocalWishlist().filter((id) => !serverIds.includes(id));
    if (missing.length === 0) return;
    void Promise.allSettled(missing.map((id) => toggleWishlistAction(id, true))).then(() =>
      router.refresh(),
    );
  }, [serverIds, router]);
  return null;
}
