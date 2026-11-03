import { Badge } from '@/components/ui/badge'

const STATUS_LABELS: Record<string, string> = {
  active: 'Active',
  converted: 'Converted',
  merged: 'Merged',
  handed_off: 'Handed off',
}

// An agent cart was built by a shopper's AI assistant over the storefront MCP endpoint - handed off, it lives on in the shopper's own cart
export function CartStatusBadges({ status, source }: { status: string; source: string }) {
  return (
    <span className="inline-flex items-center gap-1">
      <Badge variant="outline" className="text-[10px] uppercase tracking-wide">
        {STATUS_LABELS[status] ?? status}
      </Badge>
      {source === 'agent' ? (
        <Badge variant="outline" className="border-info/40 text-[10px] uppercase tracking-wide text-info" title="Built by a shopper's AI agent">
          Agent
        </Badge>
      ) : null}
    </span>
  )
}
