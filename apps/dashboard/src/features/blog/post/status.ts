export function statusLabel(status: string) {
  return status.charAt(0) + status.slice(1).toLowerCase()
}

export function statusVariant(status: string) {
  switch (status) {
    case 'PUBLISHED':
      return 'secondary'
    case 'SCHEDULED':
      return 'outline'
    case 'ARCHIVED':
      return 'destructive'
    default:
      return 'outline'
  }
}

export const STATUS_DOT: Record<string, string> = {
  PUBLISHED: 'bg-success',
  SCHEDULED: 'bg-warning',
  ARCHIVED: 'bg-destructive',
}
