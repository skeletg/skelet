import { useId } from 'react'
import { CheckCircle2Icon, CircleSlashIcon, PlusIcon, Trash2Icon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { ApplyScopePopover, type ApplyScopePopoverChannel } from './ApplyScopePopover'
import type { GroupRow, RoleDraftState, Scope } from './useRoleDraft'

const ANY = '__any__'

// GroupPanel renders the right-column body for one active permission
// group: a sticky header with bulk actions + a table of permission
// rows with inline scope pickers - Grants are toggled per-permission;
// power-users get multi-scope editing by clicking "Add scope" on a
// granted row, which expands it into a sub-list
export function GroupPanel({
  group,
  draft,
  channels,
  scopes,
}: {
  group: GroupRow
  draft: RoleDraftState
  channels: ApplyScopePopoverChannel[]
  scopes: Scope[]
}) {
  // grantedAny gates the "Apply scope to all granted" button + the bulk-action visibility - A group with zero grants in
  // this role hides "Revoke all" + "Apply scope" - there's nothing to revoke or apply to
  const grantedAny = group.permissions.some(
    (p) => (draft.scopesByPermission[p.id]?.length ?? 0) > 0,
  )

  // Index scopes by their position in the master array so the row editor can mutate the right entry - The Scope.permissionId field
  // identifies the row's permission; the index identifies the scope instance among its peers for that permission
  const indexByPermission: Record<string, number[]> = {}
  scopes.forEach((scope, index) => {
    ;(indexByPermission[scope.permissionId] ??= []).push(index)
  })

  return (
    <section className="grid gap-4">
      <header className="flex flex-wrap items-end justify-between gap-3 border-b pb-3">
        <div>
          <h2 className="text-lg font-semibold tracking-tight">{group.name}</h2>
          <p className="text-xs text-muted-foreground">
            {group.permissions.length} permission{group.permissions.length === 1 ? '' : 's'} in this group.
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => draft.grantAllInGroup(group)}
          >
            <CheckCircle2Icon className="size-4" />
            Grant all
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={!grantedAny}
            onClick={() => draft.revokeAllInGroup(group)}
          >
            <CircleSlashIcon className="size-4" />
            Revoke all
          </Button>
          <ApplyScopePopover
            channels={channels}
            disabled={!grantedAny}
            onApply={(channelId, locale) => draft.applyScopeToGroup(group, channelId, locale)}
          />
        </div>
      </header>

      <ul className="grid gap-2">
        {group.permissions.map((permission) => {
          const indices = indexByPermission[permission.id] ?? []
          const permissionScopes = indices.map((i) => scopes[i])
          const granted = permissionScopes.length > 0
          return (
            <li key={permission.id} className="rounded-md border bg-card">
              <div className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3 px-3 py-2.5">
                <div className="min-w-0">
                  <code className="block truncate font-mono text-sm" title={permission.name}>
                    {permission.slug}
                  </code>
                  <p className="truncate text-xs text-muted-foreground">{permission.name}</p>
                </div>
                <Switch
                  checked={granted}
                  onCheckedChange={(next) => draft.togglePermission(permission, next)}
                  aria-label={`Grant ${permission.slug}`}
                />
              </div>

              {granted && (
                <div className="grid gap-1.5 border-t bg-muted/20 px-3 py-2.5">
                  {permissionScopes.map((scope, localIndex) => {
                    const globalIndex = indices[localIndex]
                    return (
                      <ScopeRow
                        key={globalIndex}
                        scope={scope}
                        channels={channels}
                        onChange={(patch) => draft.updateScopeAt(globalIndex, patch)}
                        onRemove={
                          // Don't let the operator remove the last scope row - that's what the Granted switch is for;
                          // mirroring it via a delete button confuses the model
                          permissionScopes.length === 1
                            ? undefined
                            : () => draft.removeScopeAt(globalIndex)
                        }
                      />
                    )
                  })}
                  <button
                    type="button"
                    onClick={() => draft.addScope(permission.id, permission.slug)}
                    className="mt-1 inline-flex items-center gap-1 self-start text-xs text-muted-foreground hover:text-foreground"
                  >
                    <PlusIcon className="size-3" />
                    Add another scope
                  </button>
                </div>
              )}
            </li>
          )
        })}
      </ul>
    </section>
  )
}

// ScopeRow renders one (channel?, locale?) editor row - The channel + locale dropdowns each carry the "Any …" sentinel
// as the first option - selecting it writes null into the corresponding field
function ScopeRow({
  scope,
  channels,
  onChange,
  onRemove,
}: {
  scope: Scope
  channels: ApplyScopePopoverChannel[]
  onChange: (patch: { channelId?: string | null; locale?: string | null }) => void
  onRemove?: () => void
}) {
  const fid = useId()
  const channel = channels.find((c) => c.id === scope.channelId)
  const localeOptions = channel
    ? Array.from(new Set([channel.defaultLocale, ...channel.locales.map((l) => l.code)]))
    : Array.from(
        new Set(
          channels.flatMap((c) => [c.defaultLocale, ...c.locales.map((l) => l.code)]),
        ),
      )

  return (
    <div className="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_28px] items-center gap-2">
      <div className="grid gap-0.5">
        <Label htmlFor={`${fid}-channel`} className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
          Channel
        </Label>
        <Select
          value={scope.channelId === null ? ANY : scope.channelId}
          onValueChange={(v) => onChange({ channelId: v === ANY ? null : v, locale: null })}
        >
          <SelectTrigger id={`${fid}-channel`} className="h-8">
            <SelectValue placeholder="Any channel" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ANY}>Any channel</SelectItem>
            {channels.map((c) => (
              <SelectItem key={c.id} value={c.id}>
                {c.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className="grid gap-0.5">
        <Label htmlFor={`${fid}-locale`} className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
          Locale
        </Label>
        <Select
          value={scope.locale === null ? ANY : scope.locale}
          onValueChange={(v) => onChange({ locale: v === ANY ? null : v })}
        >
          <SelectTrigger id={`${fid}-locale`} className="h-8">
            <SelectValue placeholder="Any locale" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ANY}>Any locale</SelectItem>
            {localeOptions.map((l) => (
              <SelectItem key={l} value={l}>
                <span className="font-mono">{l}</span>
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className="grid items-end">
        {onRemove && (
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            onClick={onRemove}
            aria-label="Remove scope"
          >
            <Trash2Icon className="size-4" />
          </Button>
        )}
      </div>
    </div>
  )
}
