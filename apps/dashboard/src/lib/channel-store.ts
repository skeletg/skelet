let current = '' // mirrors locale-store.ts - lets apollo.ts attach `X-Channel-Id` without importing config-scope.tsx and creating a cycle

export function setEffectiveChannelId(channelId: string): void {
  current = channelId
}

export function getEffectiveChannelId(): string {
  return current // '' is Global scope, so the link omits the header and channel.AdminMiddleware reads it as "all channels", never the default store
}
