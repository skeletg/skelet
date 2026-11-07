import { Badge } from '@/components/ui/badge'

export function Outcome({ cartChanged, checkout, incomplete }: { cartChanged: boolean; checkout: boolean; incomplete: boolean }) {
  if (!cartChanged && !checkout && !incomplete) return null
  return (
    <span className="flex flex-wrap gap-1.5">
      {cartChanged ? <Badge variant="success">Filled cart</Badge> : null}
      {checkout ? <Badge variant="info">Sent to checkout</Badge> : null}
      {incomplete ? <Badge variant="warning">Cut short</Badge> : null}
    </span>
  )
}
