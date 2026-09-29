"use client";

// Client cart store for the interactive surfaces - seeded from the server render, then
// kept current by the hydrated cart every mutation returns, so no revalidate round-trip
//
// A seq guard stops a slow response overwriting a newer op, and hydrate() never adopts
// a server cart while an optimistic op is still in flight

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";

import type { Cart, CartLine } from "@/app/lib/cart/types";
import {
  clearCart as clearCartAction,
  removeCartLine,
  restoreCartLineAction,
  setCartLineQuantity,
  type RestoreLinePayload,
} from "@/app/lib/cart/actions";

function lineToRestorePayload(line: CartLine): RestoreLinePayload {
  if (line.components.length > 0) {
    const byOption = new Map<string, string[]>();
    for (const c of line.components) {
      const ids = byOption.get(c.optionId) ?? [];
      ids.push(c.selectionId);
      byOption.set(c.optionId, ids);
    }
    return {
      kind: "bundle",
      productSku: line.sku,
      quantity: line.quantity,
      selections: [...byOption].map(([optionId, selectionIds]) => ({
        optionId,
        selectionIds,
      })),
    };
  }
  if (line.options.length > 0) {
    const byOption = new Map<string, { valueIds: string[]; textValue: string }>();
    for (const o of line.options) {
      const cur = byOption.get(o.productOptionId) ?? { valueIds: [], textValue: "" };
      if (o.optionValueId) cur.valueIds.push(o.optionValueId);
      // Carry free-text for text-type options; select options snapshot an empty
      // textValue, and file options can't be re-added without a fresh upload
      if (o.textValue && o.inputType !== "file" && !o.optionValueId) {
        cur.textValue = o.textValue;
      }
      byOption.set(o.productOptionId, cur);
    }
    return {
      kind: "configured",
      productSku: line.sku,
      quantity: line.quantity,
      options: [...byOption].map(([optionId, v]) => ({
        optionId,
        valueIds: v.valueIds.length > 0 ? v.valueIds : undefined,
        textValue: v.textValue || undefined,
      })),
    };
  }
  return {
    kind: "simple",
    productSku: line.sku,
    quantity: line.quantity,
  };
}

function recomputeSubtotal(cart: Cart): Cart {
  const subtotal = cart.lines.reduce((s, l) => s + l.totalPrice, 0);
  // Scale the aggregate discount with the subtotal so the Total doesn't overshoot on a
  // +/- edit when a proportional (percentage) coupon is applied
  const totalDiscount =
    cart.subtotal > 0
      ? Math.min(subtotal, Math.round((cart.totalDiscount * subtotal) / cart.subtotal))
      : 0;
  return { ...cart, subtotal, totalDiscount };
}

export const UNDO_WINDOW_MS = 3000;

export type PendingRemoval = {
  line: CartLine;
  index: number;
  token: number;
  expiresAt: number;
};

export type CartRenderItem =
  | { type: "line"; line: CartLine }
  | { type: "removed"; pending: PendingRemoval };

export function mergePendingRemovals(
  lines: CartLine[],
  pending: PendingRemoval[],
): CartRenderItem[] {
  const items: CartRenderItem[] = lines.map((line) => ({ type: "line", line }));
  for (const p of [...pending].sort((a, b) => a.index - b.index)) {
    const at = Math.min(p.index, items.length);
    items.splice(at, 0, { type: "removed", pending: p });
  }
  return items;
}

type CartStore = {
  cart: Cart | null;
  count: number;
  // Authoritative replace: mutation return, add-to-cart feedback, hydrator
  applyServerCart: (cart: Cart | null) => void;
  setLineQuantity: (lineId: string, quantity: number) => void;
  removeLine: (lineId: string) => void;
  clear: () => void;
  pendingRemovals: PendingRemoval[];
  undoRemove: (token: number) => void;
  dismissRemoval: (token: number) => void;
  hydrate: (serverCart: Cart | null) => boolean;
  // Transient rejection feedback - per line for qty edits, cart-wide for restores
  lineNotices: Record<string, string>;
  cartNotice: string | null;
};

const Ctx = createContext<CartStore | null>(null);

export function CartStoreProvider({
  initial,
  children,
}: {
  initial: Cart | null;
  children: React.ReactNode;
}) {
  const [cart, setCart] = useState<Cart | null>(initial);
  const [pendingRemovals, setPendingRemovals] = useState<PendingRemoval[]>([]);
  const [lineNotices, setLineNotices] = useState<Record<string, string>>({});
  const [cartNotice, setCartNotice] = useState<string | null>(null);
  const noticeTimers = useRef(
    new Map<string, ReturnType<typeof setTimeout>>(),
  );
  const seq = useRef(0);
  // In-flight optimistic ops; hydrate() defers while > 0.
  const inflight = useRef(0);
  const undoToken = useRef(0);
  // Mirror of cart for synchronous reads (capturing the removed line snapshot before
  // the state update drops it)
  const cartRef = useRef(cart);
  useEffect(() => {
    cartRef.current = cart;
  }, [cart]);

  const NOTICE_MS = 6000;
  const flagLine = useCallback((lineId: string, message: string) => {
    setLineNotices((cur) => ({ ...cur, [lineId]: message }));
    const timers = noticeTimers.current;
    const existing = timers.get(lineId);
    if (existing) clearTimeout(existing);
    timers.set(
      lineId,
      setTimeout(() => {
        timers.delete(lineId);
        setLineNotices((cur) => {
          const next = { ...cur };
          delete next[lineId];
          return next;
        });
      }, NOTICE_MS),
    );
  }, []);

  const flagCart = useCallback((message: string) => {
    setCartNotice(message);
    const timers = noticeTimers.current;
    const existing = timers.get("");
    if (existing) clearTimeout(existing);
    timers.set(
      "",
      setTimeout(() => {
        timers.delete("");
        setCartNotice(null);
      }, NOTICE_MS),
    );
  }, []);

  useEffect(() => {
    const timers = noticeTimers.current;
    return () => timers.forEach(clearTimeout);
  }, []);

  const applyServerCart = useCallback((next: Cart | null) => {
    seq.current += 1; // supersede any pending optimistic reconcile
    setCart(next);
  }, []);

  const hydrate = useCallback((serverCart: Cart | null): boolean => {
    if (inflight.current > 0) return false; // don't clobber an optimistic edit
    setCart(serverCart);
    return true;
  }, []);

  const run = useCallback(
    (patch: (prev: Cart) => Cart, action: () => Promise<Cart | null>) => {
      const mySeq = (seq.current += 1);
      inflight.current += 1;
      setCart((prev) => (prev ? patch(prev) : prev));
      action()
        .then((returned) => {
          if (seq.current === mySeq && returned) setCart(returned);
        })
        .catch(() => {
          // Swallow: a failed mutation leaves the optimistic patch in place until the
          // next successful op or navigation reconciles it
        })
        .finally(() => {
          inflight.current -= 1;
        });
    },
    [],
  );

  const setLineQuantity = useCallback(
    (lineId: string, quantity: number) => {
      run(
        (prev) =>
          quantity <= 0
            ? recomputeSubtotal({
                ...prev,
                lines: prev.lines.filter((l) => l.id !== lineId),
              })
            : recomputeSubtotal({
                ...prev,
                lines: prev.lines.map((l) =>
                  l.id === lineId
                    ? {
                        ...l,
                        quantity,
                        totalPrice: Math.round(l.unitPrice * quantity),
                      }
                    : l,
                ),
              }),
        () =>
          setCartLineQuantity(lineId, quantity).then((res) => {
            // The reconcile snaps the stepper back - say why or it reads as broken
            if (res.error) flagLine(lineId, res.error);
            return res.cart;
          }),
      );
    },
    [run, flagLine],
  );

  const removeLine = useCallback(
    (lineId: string) => {
      const lines = cartRef.current?.lines ?? [];
      const index = lines.findIndex((l) => l.id === lineId);
      if (index >= 0) {
        const token = (undoToken.current += 1);
        const expiresAt = Date.now() + UNDO_WINDOW_MS;
        setPendingRemovals((cur) => [
          ...cur,
          { line: lines[index], index, token, expiresAt },
        ]);
      }
      run(
        (prev) =>
          recomputeSubtotal({
            ...prev,
            lines: prev.lines.filter((l) => l.id !== lineId),
          }),
        () => removeCartLine(lineId),
      );
    },
    [run],
  );

  const undoRemove = useCallback(
    (token: number) => {
      const pending = pendingRemovals.find((p) => p.token === token);
      if (!pending) return;
      setPendingRemovals((cur) => cur.filter((p) => p.token !== token));
      // Re-insert optimistically at the original slot, then re-add server-side and
      // adopt the authoritative return (real new line id + pricing)
      run((prev) => {
        const lines = [...prev.lines];
        lines.splice(Math.min(pending.index, lines.length), 0, pending.line);
        return recomputeSubtotal({ ...prev, lines });
      }, () =>
        restoreCartLineAction(lineToRestorePayload(pending.line)).then((res) => {
          if (res.error) {
            // File picks can't be re-added without a fresh upload - name that case
            // instead of echoing a raw required-option error
            const needsFile = pending.line.options.some(
              (o) => o.inputType === "file",
            );
            flagCart(
              needsFile
                ? "Couldn't restore - this item needs a file upload. Add it again from the product page."
                : `Couldn't restore this item - ${res.error}`,
            );
          }
          return res.cart;
        }));
    },
    [pendingRemovals, run, flagCart],
  );

  const dismissRemoval = useCallback(
    (token: number) =>
      setPendingRemovals((cur) => cur.filter((p) => p.token !== token)),
    [],
  );

  const clear = useCallback(() => {
    setPendingRemovals([]); // a whole-cart clear isn't a single-line undo
    run(
      (prev) => ({
        ...prev,
        lines: [],
        subtotal: 0,
        totalDiscount: 0,
        appliedCodes: [],
      }),
      () => clearCartAction(),
    );
  }, [run]);

  const count = cart ? cart.lines.reduce((n, l) => n + l.quantity, 0) : 0;

  const value = useMemo<CartStore>(
    () => ({
      cart,
      count,
      applyServerCart,
      setLineQuantity,
      removeLine,
      clear,
      pendingRemovals,
      undoRemove,
      dismissRemoval,
      hydrate,
      lineNotices,
      cartNotice,
    }),
    [
      cart,
      count,
      applyServerCart,
      setLineQuantity,
      removeLine,
      clear,
      pendingRemovals,
      undoRemove,
      dismissRemoval,
      hydrate,
      lineNotices,
      cartNotice,
    ],
  );

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useCartStore(): CartStore {
  const ctx = useContext(Ctx);
  if (!ctx) {
    throw new Error("useCartStore must be used inside <CartStoreProvider>");
  }
  return ctx;
}
