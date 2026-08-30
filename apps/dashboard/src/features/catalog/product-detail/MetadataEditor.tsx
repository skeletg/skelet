import { useCallback, useMemo, useState } from 'react'
import { PlusIcon, XIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Separator } from '@/components/ui/separator'
import {
  useProductSaveSection,
  type SectionDiffEntry,
} from '../productSaveRegistry'
import { added, changed, removed } from '@/shared/saving/diff'

export type MetadataEntry = { key: string; value: string }

function cleanMetadata(entries: MetadataEntry[]): MetadataEntry[] {
  return entries
    .map((entry) => ({ key: entry.key.trim(), value: entry.value }))
    .filter((entry) => entry.key || entry.value)
}

function findDuplicateKey(entries: MetadataEntry[]): string | null {
  const seen = new Set<string>()
  for (const entry of entries) {
    if (!entry.key) continue
    if (seen.has(entry.key)) return entry.key
    seen.add(entry.key)
  }
  return null
}

function metadataChanged(a: MetadataEntry[], b: MetadataEntry[]): boolean {
  return a.length !== b.length || a.some((entry, idx) => entry.key !== b[idx]?.key || entry.value !== b[idx]?.value)
}

export function ProductMetadataEditor({
  metadata,
  privateMetadata,
  canManage,
  onSave,
}: {
  metadata: MetadataEntry[]
  privateMetadata: MetadataEntry[]
  canManage: boolean
  onSave: (input: { metadata: MetadataEntry[]; privateMetadata: MetadataEntry[] }) => Promise<unknown>
}) {
  const [publicRows, setPublicRows] = useState<MetadataEntry[]>(metadata)
  const [privateRows, setPrivateRows] = useState<MetadataEntry[]>(privateMetadata)

  // Re-sync drafts when the loaded product refetches (state adjusted during
  // render, keyed on the serialized source data)
  const publicKey = JSON.stringify(metadata)
  const privateKey = JSON.stringify(privateMetadata)
  const [prevPublicKey, setPrevPublicKey] = useState(publicKey)
  const [prevPrivateKey, setPrevPrivateKey] = useState(privateKey)
  if (prevPublicKey !== publicKey) {
    setPrevPublicKey(publicKey)
    setPublicRows(metadata)
  }
  if (prevPrivateKey !== privateKey) {
    setPrevPrivateKey(privateKey)
    setPrivateRows(privateMetadata)
  }

  // Per-key rows (same shape as CollectionDetail's metadataDiffEntries): "metadata · key: old → new", plus add/remove
  // rows - a count summary hid WHICH keys changed and read identical for same-count edits
  function metadataKeyDiff(prefix: string, before: MetadataEntry[], after: MetadataEntry[]): SectionDiffEntry[] {
    const cleanedAfter = cleanMetadata(after)
    const beforeMap = new Map(before.map((e) => [e.key, e.value]))
    const afterMap = new Map(cleanedAfter.map((e) => [e.key, e.value]))
    const out: SectionDiffEntry[] = []
    for (const [key, value] of afterMap) {
      if (!beforeMap.has(key)) out.push(added(`${prefix} · ${key || '(no key)'}`, value))
      else if (beforeMap.get(key) !== value) {
        const entry = changed(`${prefix} · ${key}`, beforeMap.get(key), value)
        if (entry) out.push(entry)
      }
    }
    for (const [key, value] of beforeMap) {
      if (!afterMap.has(key)) out.push(removed(`${prefix} · ${key}`, value))
    }
    return out
  }

  const diff = useMemo<SectionDiffEntry[]>(() => {
    const entries: SectionDiffEntry[] = []
    if (metadataChanged(publicRows, metadata)) {
      entries.push(...metadataKeyDiff('metadata', metadata, publicRows))
    }
    if (metadataChanged(privateRows, privateMetadata)) {
      entries.push(...metadataKeyDiff('private metadata', privateMetadata, privateRows))
    }
    return entries
  }, [publicRows, privateRows, metadata, privateMetadata])

  const save = useCallback(async () => {
    const cleanedPublic = cleanMetadata(publicRows)
    const cleanedPrivate = cleanMetadata(privateRows)
    const missingPublicKey = cleanedPublic.some((entry) => !entry.key)
    const missingPrivateKey = cleanedPrivate.some((entry) => !entry.key)
    const publicDuplicate = findDuplicateKey(cleanedPublic)
    const privateDuplicate = findDuplicateKey(cleanedPrivate)

    if (missingPublicKey || missingPrivateKey) {
      throw new Error('Metadata entries with a value need a key.')
    }
    if (publicDuplicate) {
      throw new Error(`Duplicate metadata key: "${publicDuplicate}"`)
    }
    if (privateDuplicate) {
      throw new Error(`Duplicate private metadata key: "${privateDuplicate}"`)
    }
    await onSave({ metadata: cleanedPublic, privateMetadata: cleanedPrivate })
  }, [publicRows, privateRows, onSave])

  useProductSaveSection({ id: 'metadata', title: 'Metadata', diff, save })

  return (
    <div className="grid gap-6">
      <KeyValueMetadataEditor
        title="Metadata"
        description="Publicly exposable key-value pairs for channels, APIs, and integrations."
        rows={publicRows}
        disabled={!canManage}
        onChange={setPublicRows}
      />
      <Separator />
      <KeyValueMetadataEditor
        title="Private metadata"
        description="Internal-only key-value pairs for admin workflows and backend integrations."
        rows={privateRows}
        disabled={!canManage}
        onChange={setPrivateRows}
      />
    </div>
  )
}

function KeyValueMetadataEditor({
  title,
  description,
  rows,
  disabled,
  onChange,
}: {
  title: string
  description: string
  rows: MetadataEntry[]
  disabled: boolean
  onChange: (rows: MetadataEntry[]) => void
}) {
  function addRow() {
    onChange([...rows, { key: '', value: '' }])
  }

  function removeRow(index: number) {
    onChange(rows.filter((_, idx) => idx !== index))
  }

  function updateRow(index: number, patch: Partial<MetadataEntry>) {
    onChange(rows.map((row, idx) => (idx === index ? { ...row, ...patch } : row)))
  }

  return (
    <section className="grid gap-3">
      <div className="flex items-center justify-between gap-3">
        <div className="grid gap-1">
          <h3 className="text-sm font-medium">{title}</h3>
          <p className="text-sm text-muted-foreground">{description}</p>
        </div>
        <Button type="button" variant="outline" size="sm" onClick={addRow} disabled={disabled}>
          <PlusIcon className="size-4" />
          Add entry
        </Button>
      </div>

      {rows.length === 0 ? (
        <p className="rounded-md border border-dashed px-3 py-3 text-sm text-muted-foreground">
          No entries.
        </p>
      ) : (
        <div className="grid gap-2">
          {rows.map((row, index) => (
            <div key={index} className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto]">
              <Input
                placeholder="key"
                value={row.key}
                onChange={(event) => updateRow(index, { key: event.target.value })}
                className="font-mono text-xs"
                disabled={disabled}
              />
              <Input
                placeholder="value"
                value={row.value}
                onChange={(event) => updateRow(index, { value: event.target.value })}
                className="font-mono text-xs"
                disabled={disabled}
              />
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="size-9 justify-self-start sm:justify-self-auto"
                onClick={() => removeRow(index)}
                disabled={disabled}
                aria-label={`Remove ${title} row`}
              >
                <XIcon className="size-4" />
              </Button>
            </div>
          ))}
        </div>
      )}
    </section>
  )
}
