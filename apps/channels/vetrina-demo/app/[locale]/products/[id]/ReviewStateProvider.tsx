"use client";

import {
  createContext,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";

import {
  loadMyReviewState,
  type MyReviewState,
} from "@/app/lib/reviews/actions-client";

type State = MyReviewState & { resolved: boolean };

const EMPTY: State = { eligibility: null, votes: {}, resolved: false };

const Ctx = createContext<State>(EMPTY);

// Votes are fetched for the NEWEST page only - the page a re-sort loads carries its own
// myVote per row, and VoteRow falls back to that when a review is missing from this map
export function ReviewStateProvider({
  productId,
  children,
}: {
  productId: string;
  children: ReactNode;
}) {
  const [state, setState] = useState<State>(EMPTY);

  useEffect(() => {
    let live = true;
    loadMyReviewState(productId)
      .then((s) => live && setState({ ...s, resolved: true }))
      .catch(() => live && setState({ ...EMPTY, resolved: true }));
    return () => {
      live = false;
    };
  }, [productId]);

  return <Ctx.Provider value={state}>{children}</Ctx.Provider>;
}

export function useReviewState(): State {
  return useContext(Ctx);
}
