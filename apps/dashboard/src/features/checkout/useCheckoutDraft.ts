import { useRef, useState } from 'react'
import { useApolloClient } from '@apollo/client/react'
import { sameJson } from '@/shared/saving/diff'
import { useSaveSection } from '@/shared/saving/saveRegistry'
import {
  CreateCheckoutOptionDocument,
  DeleteCheckoutOptionDocument,
  SaveCheckoutDeliveryPointsDocument,
  UpdateCheckoutOptionDocument,
} from './Checkout.gql'
import { optionsDiff, pointsDiff } from './checkoutDiff'
import type { TabId } from './labels'
import { toOption, toPoints, type Configuration, type Option, type Point } from './types'

function optionInput(o: Option) {
  return {
    kind: o.kind,
    name: o.name,
    description: o.description,
    enabled: o.enabled,
    sortOrder: o.sortOrder,
    blockModes: o.blockModes,
    pointSource: o.pointSource,
    carrierSlug: o.carrierSlug,
  }
}

function pointInput(p: Point) {
  return {
    id: p.id || null,
    name: p.name.trim(),
    street1: p.street1,
    street2: p.street2,
    city: p.city,
    region: p.region,
    postalCode: p.postalCode,
    countryCode: p.countryCode.trim().toUpperCase(),
    instructions: p.instructions,
    hours: p.hours,
    isActive: p.isActive,
  }
}

// The page's single staged draft - every tab edits it, the header Save writes it.
// `onSaved` hands new IDs to the page; it refetches and remounts only after all sections save.
export function useCheckoutDraft({
  channelId,
  configuration,
  onSaved,
}: {
  channelId: string
  configuration: Configuration
  onSaved: (createdIds: Map<string, string>) => Promise<void>
}) {
  const client = useApolloClient()
  const [baseOptions] = useState(() => configuration.options.map(toOption))
  const [options, setOptions] = useState(baseOptions)
  const [deleted, setDeleted] = useState<string[]>([])
  const [removedCreatedOptions, setRemovedCreatedOptions] = useState<Option[]>([])
  const [basePoints] = useState(() => toPoints(configuration.deliveryPoints))
  const [points, setPoints] = useState(basePoints)
  const adopted = !configuration.usingDefaults

  // Survive a save that fails half way: a retry must not create an option, or delete one, twice
  const created = useRef(new Map<string, string>())
  const createdOptions = useRef(new Map<string, Option>())
  const deletedDone = useRef(new Set<string>())
  const savedPointIds = useRef(new Map<string, string>())

  const isNew = (id: string) => !baseOptions.some((o) => o.id === id)
  const diff = [...optionsDiff([...baseOptions, ...removedCreatedOptions], options, deleted, configuration), ...pointsDiff(basePoints, points)]

  const dirtyTabs = new Set<TabId>()
  if (deleted.length || options.some((o) => isNew(o.id))) dirtyTabs.add('delivery')
  for (const o of options) {
    const old = baseOptions.find((x) => x.id === o.id)
    if (old && !sameJson(old, o)) dirtyTabs.add('delivery')
  }
  if (!sameJson(basePoints, points)) dirtyTabs.add('points')

  async function save() {
    if (!adopted) throw new Error('Start customising checkout on the Overview tab before saving changes.')

    if (options.some((o) => !o.name.trim())) throw new Error('Every delivery option needs a name.')
    if (points.some((p) => !p.name.trim() || !p.street1.trim() || !p.city.trim() || !p.postalCode.trim() || !/^[A-Za-z]{2}$/.test(p.countryCode.trim()))) {
      throw new Error('Every pickup point needs a name, street, city, postcode and country.')
    }

    // Points first, so a pickup point option switched on in the same save finds them in its checklist
    if (!sameJson(basePoints, points)) {
      const result = await client.mutate({ mutation: SaveCheckoutDeliveryPointsDocument, variables: { channelId, points: points.map((p) => pointInput({ ...p, id: savedPointIds.current.get(p.key) ?? p.id })) } })
      result.data?.saveDeliveryPoints.forEach((p, i) => savedPointIds.current.set(points[i].key, p.id))
    }
    for (const o of options) {
      if (!isNew(o.id) || created.current.has(o.id)) continue
      // A new option starts off - it can be switched on once its setup checklist passes
      const result = await client.mutate({
        mutation: CreateCheckoutOptionDocument,
        variables: { channelId, input: { ...optionInput(o), enabled: false } },
      })
      const id = result.data?.createDeliveryOption.id
      if (!id) throw new Error(`Could not create ${o.name}`)
      created.current.set(o.id, id)
      createdOptions.current.set(o.id, o)
    }
    for (const o of options) {
      const old = baseOptions.find((x) => x.id === o.id)
      const id = created.current.get(o.id) ?? o.id
      if (created.current.has(o.id) || (old && !sameJson(old, o))) {
        await client.mutate({ mutation: UpdateCheckoutOptionDocument, variables: { id, input: optionInput(o) } })
      }
    }
    for (const draftId of deleted) {
      const id = created.current.get(draftId) ?? draftId
      if (deletedDone.current.has(id)) continue
      await client.mutate({ mutation: DeleteCheckoutOptionDocument, variables: { id } })
      deletedDone.current.add(id)
    }
    await onSaved(new Map([...created.current, ...savedPointIds.current]))
  }

  useSaveSection({ id: 'checkout', title: 'Checkout', priority: 100, diff, save })

  function patchOption(id: string, patch: Partial<Option>) {
    setOptions((prev) => prev.map((o) => (o.id === id ? { ...o, ...patch } : o)))
  }

  function addOption(kind: string): string {
    const id = crypto.randomUUID()
    const label = configuration.kinds.find((k) => k.slug === kind)?.label ?? 'Delivery'
    setOptions((prev) => [
      ...prev,
      { id, kind, name: label, description: '', enabled: false, sortOrder: prev.length, blockModes: {}, pointSource: '', carrierSlug: '' },
    ])
    return id
  }

  function deleteOption(id: string) {
    setOptions((prev) => prev.filter((o) => o.id !== id))
    if (!isNew(id) || created.current.has(id)) setDeleted((prev) => [...prev, id])
    const saved = createdOptions.current.get(id)
    if (saved) setRemovedCreatedOptions((prev) => [...prev, saved])
  }

  function addPoint(): string {
    const key = crypto.randomUUID()
    setPoints((prev) => [
      ...prev,
      {
        key,
        id: '',
        name: '',
        street1: '',
        street2: '',
        city: '',
        region: '',
        postalCode: '',
        countryCode: '',
        instructions: '',
        hours: '',
        isActive: true,
      },
    ])
    return key
  }

  function patchPoint(key: string, patch: Partial<Point>) {
    setPoints((prev) => prev.map((p) => (p.key === key ? { ...p, ...patch } : p)))
  }

  function removePoint(key: string) {
    setPoints((prev) => prev.filter((p) => p.key !== key))
  }

  return {
    adopted,
    options,
    baseOptions,
    patchOption,
    addOption,
    deleteOption,
    points,
    addPoint,
    patchPoint,
    removePoint,
    isNew,
    dirtyTabs,
  }
}

export type CheckoutDraft = ReturnType<typeof useCheckoutDraft>
