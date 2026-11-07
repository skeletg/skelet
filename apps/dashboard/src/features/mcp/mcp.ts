import { ADMIN_GRAPHQL_ENDPOINT } from '@/lib/api-endpoint'
import type { McpOverviewQuery } from './Mcp.gql'

export type Overview = McpOverviewQuery['mcpOverview']
export type Tool = Overview['admin']['tools'][number]

export function adminMcpUrl(path: string, origin = window.location.origin): string {
  return new URL(path, new URL(ADMIN_GRAPHQL_ENDPOINT, origin)).toString()
}

export function firstSentence(text: string): string {
  const m = /^(.+?[.!?])(\s|$)/s.exec(text.trim())
  return m ? m[1] : text.trim()
}

export function storefrontStatus(s: Overview['storefront']): { on: boolean; why: string } {
  if (!s.available) return { on: false, why: 'Turned off for every store on this server (MCP_FRONT_ENABLED=false).' }
  if (!s.url) return { on: false, why: "This store has no public URL, so agents can't use it: every link they hand the shopper is built from it." }
  if (!s.enabled) return { on: false, why: "Turned off in this store's settings." }
  return { on: true, why: 'Any AI agent can search this store and build a cart, with no sign-in.' }
}

export function shellQuote(value: string): string {
  return `'${value.replaceAll("'", `'\\''`)}'`
}
