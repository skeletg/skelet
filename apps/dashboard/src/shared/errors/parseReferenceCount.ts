// The "N references" count the backend puts in an in-use delete error - null when the message doesn't carry one
export function parseReferenceCount(message: string): number | null {
  const match = message.match(/(\d+)\s+references?/i)
  if (!match) return null
  const n = Number(match[1])
  return Number.isFinite(n) ? n : null
}
