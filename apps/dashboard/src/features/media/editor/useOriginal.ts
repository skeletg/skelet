import { useEffect, useState } from 'react'
import { mediaFileUrl } from '@/lib/api-endpoint'
import { fetchWithSession } from '@/lib/authed-download'

type Original = { url: string | null; error: string | null }

// The original's bytes, fetched through the API, as a local address a canvas may read back
export function useOriginal(assetId: string): Original {
  const [state, setState] = useState<Original & { id: string }>({ id: assetId, url: null, error: null })
  useEffect(() => {
    const stop = new AbortController()
    let url: string | null = null
    fetchWithSession(mediaFileUrl(assetId), { signal: stop.signal })
      .then((res) => (res.ok ? res.blob() : Promise.reject(new Error(res.status === 415 ? "This kind of file can't be edited." : `The picture couldn't be opened (HTTP ${res.status}).`))))
      .then((blob) => {
        url = URL.createObjectURL(blob)
        setState({ id: assetId, url, error: null })
      })
      .catch((e: unknown) => {
        if (!stop.signal.aborted) setState({ id: assetId, url: null, error: e instanceof Error ? e.message : "The picture couldn't be opened." })
      })
    return () => {
      stop.abort()
      if (url) URL.revokeObjectURL(url)
    }
  }, [assetId])
  return state.id === assetId ? state : { url: null, error: null }
}
