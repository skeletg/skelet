"use client";

// Client-side React Context for the current channel

import { createContext, useContext } from "react";

import type { Channel } from "@skelet/storefront-next/channel/types";

const ChannelContext = createContext<Channel | null>(null);

export function ChannelProvider({
  value,
  children,
}: {
  value: Channel | null;
  children: React.ReactNode;
}) {
  return (
    <ChannelContext.Provider value={value}>
      {children}
    </ChannelContext.Provider>
  );
}

// Nullable on purpose: a storefront page can technically render with no store resolved
// (probe contexts, misconfigured X-Channel-Id, etc)
export function useChannel(): Channel | null {
  return useContext(ChannelContext);
}
