import { getAccessToken } from '@/shared/auth/auth'

// Files move over plain HTTP rather than GraphQL - a 200MB CSV has no business being a base64 string in a
// JSON document, and the browser gets a real streamed download instead of a Blob built in memory

const BASE = '/api/admin/data'

function authHeaders(): HeadersInit {
  const token = getAccessToken()
  return token ? { authorization: `Bearer ${token}` } : {}
}

export type ImportRequest = {
  resource: string
  file: File
  format?: string
  channelId?: string
  locale?: string
  dryRun: boolean
  options: Record<string, string>
}

export async function uploadImport(req: ImportRequest): Promise<{ id: string; status: string }> {
  const body = new FormData()
  body.set('resource', req.resource)
  body.set('file', req.file)
  body.set('dryRun', String(req.dryRun))
  if (req.format) body.set('format', req.format)
  if (req.channelId) body.set('channelId', req.channelId)
  if (req.locale) body.set('locale', req.locale)
  for (const [name, value] of Object.entries(req.options)) {
    if (value !== '') body.set(`opt.${name}`, value)
  }

  const res = await fetch(`${BASE}/import`, { method: 'POST', headers: authHeaders(), body })
  if (!res.ok) throw new Error((await res.text()).trim() || `upload failed (${res.status})`)
  return res.json()
}

// Fetched rather than linked so the auth header rides along - the artifact routes are permission-gated,
// not presigned, so an anchor href would arrive unauthenticated
async function saveAs(url: string, fallbackName: string) {
  const res = await fetch(url, { headers: authHeaders() })
  if (!res.ok) throw new Error((await res.text()).trim() || `download failed (${res.status})`)
  const blob = await res.blob()
  const name = filenameFrom(res.headers.get('content-disposition')) ?? fallbackName
  const href = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = href
  a.download = name
  a.click()
  // Revoking immediately cancels the download in Safari - one tick is enough for the click to take
  setTimeout(() => URL.revokeObjectURL(href), 0)
}

function filenameFrom(header: string | null): string | null {
  const match = header?.match(/filename="([^"]+)"/)
  return match ? match[1] : null
}

export function downloadJobArtifact(jobId: string, filename: string) {
  return saveAs(`${BASE}/jobs/${jobId}/download`, filename)
}

export function downloadTemplate(resource: string, format: string, extension: string) {
  return saveAs(
    `${BASE}/template?resource=${encodeURIComponent(resource)}&format=${encodeURIComponent(format)}`,
    `${resource}-template.${extension}`,
  )
}
