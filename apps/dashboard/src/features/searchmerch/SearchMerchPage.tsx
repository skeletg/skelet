import { useMutation, useQuery } from '@apollo/client/react'
import { useState, type FormEvent, type ReactNode } from 'react'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { useConfigScope } from '@/lib/config-scope'

import {
  AddSearchSynonymDocument,
  DeleteSearchBoostDocument,
  DeleteSearchPinDocument,
  DeleteSearchRedirectDocument,
  DeleteSearchSynonymDocument,
  SearchMerchandisingDocument,
  UpsertSearchBoostDocument,
  UpsertSearchPinDocument,
  UpsertSearchRedirectDocument,
} from './SearchMerch.gql'

export function SearchMerchPage() {
  const { effectiveChannelId, channels } = useConfigScope()
  // Merchandising rules are always channel-scoped: fall back to the
  // first active channel when the header scope is global
  const channelId = effectiveChannelId ?? channels[0]?.id ?? ''
  const { data, loading, refetch } = useQuery(SearchMerchandisingDocument, {
    variables: { channelId },
    skip: !channelId,
  })

  const [addSynonym] = useMutation(AddSearchSynonymDocument)
  const [delSynonym] = useMutation(DeleteSearchSynonymDocument)
  const [upsertPin] = useMutation(UpsertSearchPinDocument)
  const [delPin] = useMutation(DeleteSearchPinDocument)
  const [upsertRedirect] = useMutation(UpsertSearchRedirectDocument)
  const [delRedirect] = useMutation(DeleteSearchRedirectDocument)
  const [upsertBoost] = useMutation(UpsertSearchBoostDocument)
  const [delBoost] = useMutation(DeleteSearchBoostDocument)

  const [error, setError] = useState<string | null>(null)
  const [synTerms, setSynTerms] = useState('')
  const [pinQuery, setPinQuery] = useState('')
  const [pinProduct, setPinProduct] = useState('')
  const [pinPosition, setPinPosition] = useState('1')
  const [redirQuery, setRedirQuery] = useState('')
  const [redirTarget, setRedirTarget] = useState('')
  const [boostQuery, setBoostQuery] = useState('')
  const [boostProduct, setBoostProduct] = useState('')
  const [boostWeight, setBoostWeight] = useState('5')

  if (!channelId) {
    return (
      <DetailPageLayout>
        <DetailPageHeader title="Search merchandising" meta="Pick a channel in the header switcher first." />
      </DetailPageLayout>
    )
  }

  const run = (fn: () => Promise<unknown>) => {
    setError(null)
    void fn()
      .then(() => refetch())
      .catch((err: unknown) =>
        setError(err instanceof Error ? err.message : 'Operation failed.'),
      )
  }
  const submit = (fn: () => Promise<unknown>) => (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    run(fn)
  }

  const merch = data?.searchMerchandising

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Search merchandising"
        meta="Synonym groups expand queries at search time, pins force products to the top of a query's results, redirects send a query straight to a URL, and boosts nudge ranking up or down."
      />
      <DetailPageBody>
        {error && <p className="mb-3 text-sm text-destructive">{error}</p>}
        {loading && !merch ? (
          <p className="text-sm text-muted-foreground">Loading…</p>
        ) : (
          <>
            <DetailPageSection
              title="Synonyms"
              description='Comma-separated equivalence groups, e.g. "tyre, tire, tires". A search for any term also matches the others - no reindex needed.'
            >
              <div className="grid gap-2">
                <RuleList
                  empty="No synonym groups yet."
                  rows={(merch?.synonyms ?? []).map((s) => ({
                    key: s.id,
                    label: <span className="font-mono text-xs">{s.terms}</span>,
                    onDelete: () =>
                      run(() => delSynonym({ variables: { channelId, id: s.id } })),
                  }))}
                />
                <form
                  onSubmit={submit(() =>
                    addSynonym({ variables: { channelId, terms: synTerms } }).then(() =>
                      setSynTerms(''),
                    ),
                  )}
                  className="flex gap-2"
                >
                  <Input
                    required
                    placeholder="tyre, tire, tires"
                    value={synTerms}
                    onChange={(e) => setSynTerms(e.target.value)}
                  />
                  <Button type="submit" variant="outline">
                    Add group
                  </Button>
                </form>
              </div>
            </DetailPageSection>

            <DetailPageSection
              title="Pinned products"
              description="Force a product to a position at the top of one query's first page. Pins key on the exact (normalized) query."
            >
              <div className="grid gap-2">
                <RuleList
                  empty="No pins yet."
                  rows={(merch?.pins ?? []).map((p) => ({
                    key: p.id,
                    label: (
                      <span>
                        <span className="font-mono text-xs">“{p.query}”</span> → product{' '}
                        <span className="font-mono text-xs">{p.productId}</span> at #{p.position}
                      </span>
                    ),
                    onDelete: () => run(() => delPin({ variables: { channelId, id: p.id } })),
                  }))}
                />
                <form
                  onSubmit={submit(() =>
                    upsertPin({
                      variables: {
                        channelId,
                        query: pinQuery,
                        productId: pinProduct.trim(),
                        position: Number.parseInt(pinPosition || '1', 10),
                      },
                    }).then(() => {
                      setPinQuery('')
                      setPinProduct('')
                      setPinPosition('1')
                    }),
                  )}
                  className="grid grid-cols-[1fr_1fr_5rem_auto] gap-2"
                >
                  <Input required placeholder="Query" value={pinQuery} onChange={(e) => setPinQuery(e.target.value)} />
                  <Input required placeholder="Product ID" value={pinProduct} onChange={(e) => setPinProduct(e.target.value)} />
                  <Input type="number" min="1" value={pinPosition} onChange={(e) => setPinPosition(e.target.value)} />
                  <Button type="submit" variant="outline">
                    Pin
                  </Button>
                </form>
                <p className="text-xs text-muted-foreground">
                  Copy the product id from the product detail page URL.
                </p>
              </div>
            </DetailPageSection>

            <DetailPageSection
              title="Redirects"
              description='Send a query straight to a page instead of results - "help" → /pages/faq, campaign terms → a landing page.'
            >
              <div className="grid gap-2">
                <RuleList
                  empty="No redirects yet."
                  rows={(merch?.redirects ?? []).map((r) => ({
                    key: r.id,
                    label: (
                      <span>
                        <span className="font-mono text-xs">“{r.query}”</span> →{' '}
                        <span className="font-mono text-xs">{r.targetUrl}</span>
                      </span>
                    ),
                    onDelete: () =>
                      run(() => delRedirect({ variables: { channelId, id: r.id } })),
                  }))}
                />
                <form
                  onSubmit={submit(() =>
                    upsertRedirect({
                      variables: { channelId, query: redirQuery, targetUrl: redirTarget.trim() },
                    }).then(() => {
                      setRedirQuery('')
                      setRedirTarget('')
                    }),
                  )}
                  className="grid grid-cols-[1fr_1fr_auto] gap-2"
                >
                  <Input required placeholder="Query" value={redirQuery} onChange={(e) => setRedirQuery(e.target.value)} />
                  <Input required placeholder="/pages/faq" value={redirTarget} onChange={(e) => setRedirTarget(e.target.value)} />
                  <Button type="submit" variant="outline">
                    Add redirect
                  </Button>
                </form>
              </div>
            </DetailPageSection>
            <DetailPageSection
              title="Boosts & buries"
              description="Nudge a product up (positive weight) or down (negative) in one query's ranking. Weights are relative multipliers, not positions: +10 roughly doubles the score, -10 roughly halves it. Zero isn't allowed."
            >
              <div className="grid gap-2">
                <RuleList
                  empty="No boosts yet."
                  rows={(merch?.boosts ?? []).map((b) => ({
                    key: b.id,
                    label: (
                      <span>
                        <span className="font-mono text-xs">“{b.query}”</span> → product{' '}
                        <span className="font-mono text-xs">{b.productId}</span>{' '}
                        <span className={b.weight > 0 ? 'text-emerald-600 dark:text-emerald-400' : 'text-destructive'}>
                          {b.weight > 0 ? `boost +${b.weight}` : `bury ${b.weight}`}
                        </span>
                      </span>
                    ),
                    onDelete: () => run(() => delBoost({ variables: { channelId, id: b.id } })),
                  }))}
                />
                <form
                  onSubmit={submit(() =>
                    upsertBoost({
                      variables: {
                        channelId,
                        query: boostQuery,
                        productId: boostProduct.trim(),
                        weight: Number.parseFloat(boostWeight || '0'),
                      },
                    }).then(() => {
                      setBoostQuery('')
                      setBoostProduct('')
                      setBoostWeight('5')
                    }),
                  )}
                  className="grid grid-cols-[1fr_1fr_6rem_auto] gap-2"
                >
                  <Input required placeholder="Query" value={boostQuery} onChange={(e) => setBoostQuery(e.target.value)} />
                  <Input required placeholder="Product ID" value={boostProduct} onChange={(e) => setBoostProduct(e.target.value)} />
                  <Input type="number" step="0.5" placeholder="+5 / -5" value={boostWeight} onChange={(e) => setBoostWeight(e.target.value)} />
                  <Button type="submit" variant="outline">
                    Add
                  </Button>
                </form>
              </div>
            </DetailPageSection>
          </>
        )}
      </DetailPageBody>
    </DetailPageLayout>
  )
}

function RuleList({
  rows,
  empty,
}: {
  rows: { key: string; label: ReactNode; onDelete: () => void }[]
  empty: string
}) {
  if (rows.length === 0) {
    return <p className="text-sm text-muted-foreground">{empty}</p>
  }
  return (
    <ul className="grid gap-1">
      {rows.map((r) => (
        <li
          key={r.key}
          className="flex items-center justify-between rounded border border-border px-3 py-1.5 text-sm"
        >
          {r.label}
          <Button variant="ghost" size="sm" onClick={r.onDelete}>
            Delete
          </Button>
        </li>
      ))}
    </ul>
  )
}
