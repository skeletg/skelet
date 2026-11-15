import { CheckIcon, SignpostIcon } from 'lucide-react'
import { useQuery } from '@apollo/client/react'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import type { OrderStatus } from '@/graphql/schema.types'
import { OrderStagesDocument } from './Orders.gql'

type Props = {
  channelId: string
  status: OrderStatus
  stage: string | null
  disabled?: boolean
  running?: boolean
  onMove: (stage: string | null) => void
}

// Moves the order between the stages its plugins offer for its status - an action like Hold, not a staged edit
export function OrderStageMenu({ channelId, status, stage, disabled, running, onMove }: Props) {
  const { data } = useQuery(OrderStagesDocument, { variables: { channelId } })
  const fitting = (data?.orderStages ?? []).filter((s) => s.within.includes(status))
  if (fitting.length === 0 && !stage) return null

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="outline" size="sm" disabled={disabled}>
          <SignpostIcon className="size-4" />
          {running ? 'Moving…' : 'Stage'}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-48">
        <DropdownMenuLabel className="text-xs font-normal text-muted-foreground">Move to stage</DropdownMenuLabel>
        {fitting.map((s) => (
          <DropdownMenuItem key={s.key} disabled={s.key === stage} onSelect={() => onMove(s.key)}>
            <CheckIcon className={s.key === stage ? 'size-4' : 'size-4 opacity-0'} />
            {s.label}
          </DropdownMenuItem>
        ))}
        {stage && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuItem onSelect={() => onMove(null)}>
              <CheckIcon className="size-4 opacity-0" />
              No stage
            </DropdownMenuItem>
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
