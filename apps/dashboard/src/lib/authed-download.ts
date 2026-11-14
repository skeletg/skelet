import { useState } from 'react'
import { ADMIN_DOWNLOAD_TICKET_ENDPOINT } from '@/lib/api-endpoint'
import { getAccessToken, setAuthStoredState } from '@/shared/auth/auth'
import { refreshSession } from '@/shared/auth/refresh'

function filenameFrom(header: string | null): string | null {
  const match = header?.match(/filename="([^"]+)"/)
  return match ? match[1]! : null
}

type Init = Omit<RequestInit, 'headers'> & { headers?: Record<string, string> }

const signed = (url: string, init: Init) => {
  const token = getAccessToken()
  return fetch(url, { ...init, headers: { ...init.headers, ...(token ? { authorization: `Bearer ${token}` } : {}) } })
}

// A plain fetch to an admin route with the session on it, renewed once if it has run out - the way Apollo does it
export async function fetchWithSession(url: string, init: Init = {}): Promise<Response> {
  const res = await signed(url, init)
  if (res.status !== 401) return res
  if (!(await refreshSession())) {
    setAuthStoredState(null)
    return res
  }
  return signed(url, init)
}

const failed = async (res: Response) => new Error((await res.text()).trim() || `download failed (${res.status})`)

// The browser saves the file straight to disk: the session buys a short-lived link for this one file, the link is tried
// so a refusal can be shown here, and then it is opened as a download - the file never passes through the page's memory
export async function downloadWithSession(url: string, fallbackName: string) {
  const target = new URL(url, window.location.href)
  const minted = await fetchWithSession(ADMIN_DOWNLOAD_TICKET_ENDPOINT, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ path: target.pathname + target.search }),
  })
  if (!minted.ok) throw await failed(minted)
  const link = new URL(((await minted.json()) as { url: string }).url, target).href
  const trying = new AbortController()
  const res = await fetch(link, { signal: trying.signal })
  if (!res.ok) throw await failed(res)
  trying.abort()
  const a = document.createElement('a')
  a.href = link
  a.download = filenameFrom(res.headers.get('content-disposition')) ?? fallbackName
  a.click()
}

// A download a button shows as under way, refusing a second click
export function useDownload(onError: (e: unknown, fallbackName: string) => void) {
  const [busy, setBusy] = useState<string | null>(null)
  const start = (url: string, fallbackName: string) => {
    if (busy) return
    setBusy(url)
    void downloadWithSession(url, fallbackName)
      .catch((e: unknown) => onError(e, fallbackName))
      .finally(() => setBusy(null))
  }
  return { busy, start }
}
