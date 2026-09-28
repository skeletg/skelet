import { useState, type FormEvent } from 'react'
import { useMutation } from '@apollo/client/react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { SubscriptionInterval } from '@/graphql/schema.types'
import {
  ClearSubscriptionTermsDocument,
  ProductDocument,
  SetSubscriptionTermsDocument,
  type SubscriptionTermsFieldsFragment,
} from './Catalog.gql'

type WithTerms = {
  id: string
  subscriptionTerms?: SubscriptionTermsFieldsFragment | null
}

const INTERVALS: { value: SubscriptionInterval; label: string }[] = [
  { value: 'DAY', label: 'Day(s)' },
  { value: 'WEEK', label: 'Week(s)' },
  { value: 'MONTH', label: 'Month(s)' },
  { value: 'YEAR', label: 'Year(s)' },
]

export function SubscriptionTermsSection({
  product,
  canManage,
}: {
  product: WithTerms
  canManage: boolean
}) {
  const terms = product.subscriptionTerms ?? null

  const [intervalUnit, setIntervalUnit] = useState<SubscriptionInterval>(
    terms?.intervalUnit ?? 'MONTH',
  )
  const [intervalCount, setIntervalCount] = useState(String(terms?.intervalCount ?? 1))
  const [trialDays, setTrialDays] = useState(String(terms?.trialDays ?? 0))
  const [error, setError] = useState<string | null>(null)

  const refetchQueries = [{ query: ProductDocument, variables: { id: product.id } }]
  const [setTerms, { loading: saving }] = useMutation(SetSubscriptionTermsDocument, { refetchQueries })
  const [clearTerms, { loading: clearing }] = useMutation(ClearSubscriptionTermsDocument, { refetchQueries })

  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setError(null)
    try {
      await setTerms({
        variables: {
          input: {
            productId: product.id,
            intervalUnit,
            intervalCount: Math.max(1, Number(intervalCount) || 1),
            trialDays: Math.max(0, Number(trialDays) || 0),
          },
        },
      })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Save failed.')
    }
  }

  async function handleClear() {
    setError(null)
    try {
      await clearTerms({ variables: { productId: product.id } })
      setIntervalUnit('MONTH')
      setIntervalCount('1')
      setTrialDays('0')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Clear failed.')
    }
  }

  return (
    <div className="grid gap-5">
      <div className="grid gap-1">
        <p className="text-sm text-muted-foreground">
          Recurring billing terms. Buying this product starts a subscription that re-bills every
          interval at the product's price. A background scheduler advances billing periods and
          records an invoice per cycle.
        </p>
        {terms ? (
          <p className="text-sm">
            Current:{' '}
            <span className="font-medium">
              every {terms.intervalCount} {terms.intervalUnit.toLowerCase()}
              {terms.intervalCount === 1 ? '' : 's'}
            </span>
            {terms.trialDays > 0 ? ` · ${terms.trialDays}-day trial` : ''}
          </p>
        ) : (
          <p className="text-sm text-muted-foreground">No terms configured yet.</p>
        )}
      </div>

      <form onSubmit={handleSubmit} className="grid gap-4">
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <div className="grid gap-2">
            <Label htmlFor="sub-count">Every</Label>
            <Input
              id="sub-count"
              type="number"
              min={1}
              value={intervalCount}
              onChange={(e) => setIntervalCount(e.target.value)}
              disabled={!canManage || saving}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="sub-unit">Interval</Label>
            <Select
              value={intervalUnit}
              onValueChange={(v) => setIntervalUnit(v as SubscriptionInterval)}
              disabled={!canManage || saving}
            >
              <SelectTrigger className="w-full" id="sub-unit">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {INTERVALS.map((i) => (
                  <SelectItem key={i.value} value={i.value}>
                    {i.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="grid gap-2">
            <Label htmlFor="sub-trial">Trial days</Label>
            <Input
              id="sub-trial"
              type="number"
              min={0}
              value={trialDays}
              onChange={(e) => setTrialDays(e.target.value)}
              disabled={!canManage || saving}
            />
          </div>
        </div>

        {error && (
          <Alert variant="destructive">
            <AlertTitle>Action failed</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        {canManage && (
          <div className="flex gap-2">
            <Button type="submit" disabled={saving}>
              {saving ? 'Saving…' : terms ? 'Update terms' : 'Set terms'}
            </Button>
            {terms && (
              <Button type="button" variant="outline" onClick={handleClear} disabled={clearing}>
                {clearing ? 'Clearing…' : 'Clear'}
              </Button>
            )}
          </div>
        )}
      </form>
    </div>
  )
}
