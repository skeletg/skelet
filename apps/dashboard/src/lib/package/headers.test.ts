import { describe, expect, it } from 'vitest'
import { originsFrom, renderHeaders } from '../../../package/headers.js'

const template = "  Content-Security-Policy: default-src 'self'; connect-src 'self' {CONNECT_ORIGINS}; frame-src 'none'; base-uri 'self'\n"

describe('the dashboard build’s security headers', () => {
  it('lets the page reach what its settings point it at, and frame the storefront it previews', () => {
    const origins = originsFrom({
      VITE_GRAPHQL_URL: 'https://api.example.com/api/admin/x1/graphql',
      VITE_FRONT_GRAPHQL_URL: 'https://api.example.com/api/front/graphql',
      VITE_STOREFRONT_ORIGIN: 'https://shop.example.com/',
      VITE_SENTRY_DSN: 'https://key@o1.ingest.example.net/42',
      UPLOAD_ORIGINS: 'https://media.example.com, https://cdn.example.com',
    })
    expect(origins.connect).toEqual([
      'https://api.example.com',
      'wss://api.example.com', // subscriptions - an https source doesn't cover them
      'https://o1.ingest.example.net',
      'https://media.example.com',
      'https://cdn.example.com',
    ])
    expect(origins.frame).toEqual(['https://shop.example.com'])
    expect(renderHeaders(template, origins.connect, [...origins.frame, 'https://maps.example.org'])).toBe(
      "  Content-Security-Policy: default-src 'self'; connect-src 'self' https://api.example.com wss://api.example.com " +
        "https://o1.ingest.example.net https://media.example.com https://cdn.example.com; frame-src https://shop.example.com " +
        "https://maps.example.org; base-uri 'self'\n",
    )
  })

  it('adds nothing for an API on the page’s own origin and no storefront', () => {
    const origins = originsFrom({ VITE_GRAPHQL_URL: '/api/admin/graphql' })
    expect(origins).toEqual({ connect: [], frame: [] })
    expect(renderHeaders(template, origins.connect, origins.frame)).toBe(template.replace(' {CONNECT_ORIGINS}', ''))
  })

  it('fails when the template stops carrying what it fills in', () => {
    expect(() => renderHeaders("connect-src 'self'; frame-src 'none'", [], [])).toThrow(/CONNECT_ORIGINS/)
    expect(() => renderHeaders("connect-src 'self' {CONNECT_ORIGINS}", [], [])).toThrow(/frame-src/)
  })
})
