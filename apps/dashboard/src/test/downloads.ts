import { vi } from 'vitest'

type Init = { method?: string; headers?: Record<string, string>; body?: string }

// The server end of a dashboard download: the ticket route hands back the path it was asked for with a ticket on it, and
// everything else - that link, or any other request - is answered by `answer`. Saved files are what the page clicked
export function downloadServer(answer: (url: string) => Response) {
  const asked: { url: string; method: string; authorization?: string }[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init: Init = {}) => {
      asked.push({ url, method: init.method ?? 'GET', authorization: init.headers?.authorization })
      if (url.endsWith('/download-ticket')) {
        const { path } = JSON.parse(init.body ?? '{}') as { path: string }
        return Response.json({ url: `${path}${path.includes('?') ? '&' : '?'}ticket=t1` })
      }
      return answer(url)
    }),
  )
  const saved: { href: string; name: string }[] = []
  vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) {
    saved.push({ href: new URL(this.href).pathname + new URL(this.href).search, name: this.download })
  })
  return { asked, saved }
}
