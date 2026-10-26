import { useCallback, useEffect, useMemo, useRef, useState, type ClipboardEvent, type DragEvent } from 'react'
import { useApolloClient, useMutation, useQuery } from '@apollo/client/react'
import {
  FolderPlusIcon,
  ImagesIcon,
  LayoutGridIcon,
  ListIcon,
  SearchIcon,
  UploadCloudIcon,
  UploadIcon,
  XIcon,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import type { MediaAssetSort, MediaMimeGroup } from '@/graphql/schema.types'
import { usePermissions } from '@/shared/auth/usePermissions'
import { useDebounced } from '@/shared/hooks/useDebounced'
import { buildDirectoryTree } from '../directory-paths'
import { DirectoryTree, RootRow } from '../directory-tree'
import {
  AllMediaDirectoriesDocument,
  CreateMediaDirectoryDocument,
  MediaAssetByIdDocument,
  SearchMediaAssetsDocument,
  type MediaAssetFieldsFragment as MediaAsset,
} from '../MediaLibrary.gql'
import { AssetGrid, Thumb, type GridSize } from './AssetGrid'
import { AssetPanel } from './AssetPanel'
import { useUploadQueue } from './useUploadQueue'

export type MediaPickerProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSelect?: (asset: MediaAsset) => void
  // Several, in the order they were picked - set multiple too
  multiple?: boolean
  onSelectMany?: (assets: MediaAsset[]) => void
  accept?: 'image' | 'document' | 'any'
  uploadPath?: string
  title?: string
  // Already in use - shown picked when the picker opens
  selectedIds?: readonly string[]
}

const PAGE = 48
const SORTS: { value: MediaAssetSort; label: string }[] = [
  { value: 'NEWEST', label: 'Newest first' },
  { value: 'OLDEST', label: 'Oldest first' },
  { value: 'NAME', label: 'Name A-Z' },
  { value: 'LARGEST', label: 'Largest first' },
]
const PREFS_KEY = 'media-picker:view'

function readPrefs(): { view: 'grid' | 'list'; size: GridSize } {
  try {
    const raw = JSON.parse(localStorage.getItem(PREFS_KEY) ?? '{}') as { view?: string; size?: string }
    return { view: raw.view === 'list' ? 'list' : 'grid', size: raw.size === 'small' ? 'small' : 'large' }
  } catch {
    return { view: 'grid', size: 'large' }
  }
}

// The one place the dashboard picks from the media library - its folders, search, uploads and details in one modal
export function MediaPickerDialog(props: MediaPickerProps) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className="flex h-[min(92vh,58rem)] flex-col gap-0 overflow-hidden p-0 sm:max-w-[min(96vw,88rem)]">
        {props.open ? <PickerBody {...props} /> : null}
      </DialogContent>
    </Dialog>
  )
}

function PickerBody({ onOpenChange, onSelect, multiple = false, onSelectMany, accept = 'image', uploadPath = 'uploads', title, selectedIds = [] }: MediaPickerProps) {
  const { has } = usePermissions()
  const canManage = has('media.manage')
  const client = useApolloClient()
  const [folder, setFolder] = useState('')
  const [search, setSearch] = useState('')
  const query = useDebounced(search.trim(), 250)
  const [type, setType] = useState<MediaMimeGroup | 'ALL'>(accept === 'document' ? 'DOCUMENT' : accept === 'image' ? 'IMAGE' : 'ALL')
  const [sort, setSort] = useState<MediaAssetSort>('NEWEST')
  const [prefs, setPrefs] = useState(readPrefs)
  const [picked, setPicked] = useState<string[]>(() => [...selectedIds])
  const [focusedId, setFocusedId] = useState<string | null>(selectedIds[0] ?? null)
  const [anchorId, setAnchorId] = useState<string | null>(null)
  // Assets met along the way - what's picked stays known when a search or folder change hides it
  const [known, setKnown] = useState<Record<string, MediaAsset>>({})
  const [dragging, setDragging] = useState(false)
  const [newFolder, setNewFolder] = useState<string | null>(null)
  const fileInput = useRef<HTMLInputElement>(null)
  const sentinel = useRef<HTMLDivElement>(null)

  const savePrefs = (next: typeof prefs) => {
    setPrefs(next)
    try {
      localStorage.setItem(PREFS_KEY, JSON.stringify(next))
    } catch {
      // A private window keeps its choice for this visit only
    }
  }

  const dirs = useQuery(AllMediaDirectoriesDocument)
  const tree = useMemo(() => buildDirectoryTree((dirs.data?.allMediaDirectories ?? []).map((d) => d.path)), [dirs.data])
  const variables = { query: query || null, path: folder || null, mimeGroup: type === 'ALL' ? null : type, sort, limit: PAGE, offset: 0 }
  const results = useQuery(SearchMediaAssetsDocument, { variables, fetchPolicy: 'cache-and-network', notifyOnNetworkStatusChange: true })
  const items = useMemo(() => results.data?.searchMediaAssets.items ?? results.previousData?.searchMediaAssets.items ?? [], [results.data, results.previousData])
  const total = results.data?.searchMediaAssets.totalCount ?? items.length
  // What's already in use when the picker opens may sit outside the first page
  const current = useQuery(MediaAssetByIdDocument, { variables: { id: selectedIds[0] ?? '' }, skip: !selectedIds[0] }).data?.mediaAsset
  const byId = useMemo(
    () => ({ ...(current ? { [current.id]: current } : {}), ...known, ...Object.fromEntries(items.map((a) => [a.id, a])) }),
    [current, known, items],
  )

  // Page by offset - the server caps one search at 200, so asking for a longer first page stalled there.
  // A page that brings nothing new (a delete since the count) ends it, or the sentinel would ask forever
  const pageKey = `${JSON.stringify(variables)}@${items.length}`
  const [stalledAt, setStalledAt] = useState<string | null>(null)
  const hasMore = items.length < total && stalledAt !== pageKey
  const asking = useRef(false)
  const { fetchMore } = results
  const loadMore = useCallback(async () => {
    if (asking.current) return
    asking.current = true
    const seen = new Set(items.map((a) => a.id))
    try {
      const res = await fetchMore({
        variables: { offset: items.length },
        updateQuery: (prev, { fetchMoreResult }) => ({
          searchMediaAssets: {
            ...fetchMoreResult.searchMediaAssets,
            items: [...prev.searchMediaAssets.items, ...fetchMoreResult.searchMediaAssets.items.filter((a) => !seen.has(a.id))],
          },
        }),
      })
      if (!res.data?.searchMediaAssets.items.some((a) => !seen.has(a.id))) setStalledAt(pageKey)
    } catch {
      setStalledAt(pageKey)
    } finally {
      asking.current = false
    }
  }, [fetchMore, items, pageKey])

  useEffect(() => {
    const el = sentinel.current
    if (!el || !hasMore || results.loading) return
    const watch = new IntersectionObserver(([entry]) => entry?.isIntersecting && void loadMore(), { rootMargin: '200px' })
    watch.observe(el)
    return () => watch.disconnect()
  }, [hasMore, results.loading, loadMore])

  const pickable = useCallback(
    (a: MediaAsset) => {
      if (a.status !== 'READY') return 'Still uploading'
      if (accept === 'image' && a.mimeGroup !== 'IMAGE') return 'Only images can be used here'
      if (accept === 'document' && a.mimeGroup !== 'DOCUMENT') return 'Only documents can be used here'
      return null
    },
    [accept],
  )
  const remember = useCallback((assets: MediaAsset[]) => setKnown((k) => ({ ...k, ...Object.fromEntries(assets.map((a) => [a.id, a])) })), [])

  const toggle = (a: MediaAsset, { range, additive }: { range: boolean; additive: boolean }) => {
    remember([a])
    setFocusedId(a.id)
    if (!multiple) {
      setPicked([a.id])
      return
    }
    if (range && anchorId) {
      const from = items.findIndex((x) => x.id === anchorId)
      const to = items.findIndex((x) => x.id === a.id)
      if (from >= 0 && to >= 0) {
        const span = items.slice(Math.min(from, to), Math.max(from, to) + 1).filter((x) => !pickable(x))
        remember(span)
        setPicked((p) => [...p, ...span.map((x) => x.id).filter((id) => !p.includes(id))])
        return
      }
    }
    setAnchorId(a.id)
    // A plain click adds to the picks here as well - Ctrl only matters for the anchor a Shift range starts from
    setPicked((p) => (p.includes(a.id) ? p.filter((id) => id !== a.id) : [...p, a.id]))
    if (!additive) setAnchorId(a.id)
  }

  const finish = (ids: string[]) => {
    const assets = ids.map((id) => byId[id]).filter((a): a is MediaAsset => !!a)
    if (assets.length === 0) return
    if (multiple) onSelectMany?.(assets)
    else onSelect?.(assets[0])
    onOpenChange(false)
  }

  const { refetch } = results
  const { refetch: refetchDirs } = dirs
  const uploads = useUploadQueue(
    folder || uploadPath,
    useCallback(
      async (ids: string[]) => {
        // The list and folders refresh on the side - a failed refresh, or the picker closing, mustn't lose the pick
        void refetch().catch(() => {})
        void refetchDirs().catch(() => {})
        const fetched = await Promise.all(
          ids.map((id) => client.query({ query: MediaAssetByIdDocument, variables: { id }, fetchPolicy: 'network-only' }).catch(() => null)),
        )
        const assets = fetched.map((r) => r?.data?.mediaAsset).filter((a): a is MediaAsset => !!a)
        remember(assets)
        const usable = assets.filter((a) => !pickable(a)).map((a) => a.id)
        if (usable.length > 0) {
          setFocusedId(usable[usable.length - 1])
          setPicked((p) => (multiple ? [...p, ...usable.filter((id) => !p.includes(id))] : [usable[usable.length - 1]]))
        }
      },
      [refetch, refetchDirs, client, remember, pickable, multiple],
    ),
  )
  const acceptFile = (file: File) => {
    if (accept === 'image' && !file.type.startsWith('image/')) return 'Not an image - only pictures can be used here'
    return null
  }
  const addFiles = (files: File[]) => {
    if (!canManage || files.length === 0) return
    void uploads.add(files, acceptFile)
  }
  const onDrop = (e: DragEvent) => {
    e.preventDefault()
    setDragging(false)
    addFiles(Array.from(e.dataTransfer.files))
  }
  const onPaste = (e: ClipboardEvent) => {
    const files = Array.from(e.clipboardData.files)
    if (files.length > 0) {
      e.preventDefault()
      addFiles(files)
    }
  }

  const [createDir, { loading: creating }] = useMutation(CreateMediaDirectoryDocument, { refetchQueries: [{ query: AllMediaDirectoriesDocument }] })
  const submitFolder = async () => {
    const name = newFolder?.trim()
    if (!name) return setNewFolder(null)
    const res = await createDir({ variables: { path: folder, name } }).catch(() => null)
    setNewFolder(null)
    if (res?.data?.createMediaDirectory) setFolder(res.data.createMediaDirectory.path)
  }

  const focused = focusedId ? byId[focusedId] ?? null : null
  const filtered = !!query || type !== (accept === 'document' ? 'DOCUMENT' : accept === 'image' ? 'IMAGE' : 'ALL')
  const pickedAssets = picked.map((id) => byId[id]).filter((a): a is MediaAsset => !!a)

  return (
    <div
      className="relative flex min-h-0 flex-1 flex-col"
      onPaste={onPaste}
      onDragEnter={(e) => {
        if (canManage && e.dataTransfer.types.includes('Files')) {
          e.preventDefault()
          setDragging(true)
        }
      }}
      onDragOver={(e) => canManage && e.dataTransfer.types.includes('Files') && e.preventDefault()}
      onDragLeave={(e) => {
        if (e.currentTarget === e.target) setDragging(false)
      }}
      onDrop={onDrop}
    >
      <DialogHeader className="gap-3 border-b px-5 py-4">
        <div className="flex flex-wrap items-center justify-between gap-3 pr-8">
          <div className="grid gap-0.5">
            <DialogTitle className="flex items-center gap-2">
              <ImagesIcon className="size-5 text-muted-foreground" />
              {title ?? (accept === 'image' ? 'Choose a picture' : 'Choose from the media library')}
            </DialogTitle>
            <DialogDescription>
              {canManage ? 'Pick from the library, or drop, paste or upload new files.' : 'Pick from the media library.'}
              {multiple ? ' Pick several with Ctrl or Shift.' : ' Double-click to use one right away.'}
            </DialogDescription>
          </div>
          {canManage ? (
            <Button type="button" size="sm" onClick={() => fileInput.current?.click()} disabled={uploads.busy}>
              <UploadIcon className="size-4" />
              Upload
            </Button>
          ) : null}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <div className="relative min-w-[14rem] flex-1">
            <SearchIcon className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search names, titles, alt text and captions…"
              aria-label="Search media"
              className="h-9 pl-8 pr-8"
              autoFocus
            />
            {search ? (
              <button type="button" aria-label="Clear search" onClick={() => setSearch('')} className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground">
                <XIcon className="size-4" />
              </button>
            ) : null}
          </div>
          {accept === 'any' ? (
            <Select value={type} onValueChange={(v) => setType(v as MediaMimeGroup | 'ALL')}>
              <SelectTrigger size="sm" className="w-[9rem]" aria-label="Type">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="ALL">Everything</SelectItem>
                <SelectItem value="IMAGE">Images</SelectItem>
                <SelectItem value="DOCUMENT">Documents</SelectItem>
              </SelectContent>
            </Select>
          ) : null}
          <Select value={sort} onValueChange={(v) => setSort(v as MediaAssetSort)}>
            <SelectTrigger size="sm" className="w-[9.5rem]" aria-label="Sort">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {SORTS.map((s) => (
                <SelectItem key={s.value} value={s.value}>{s.label}</SelectItem>
              ))}
            </SelectContent>
          </Select>
          <div className="flex items-center rounded-md border p-0.5" role="group" aria-label="View">
            <Button type="button" variant={prefs.view === 'grid' ? 'secondary' : 'ghost'} size="icon" className="size-7" aria-pressed={prefs.view === 'grid'} aria-label="Grid" onClick={() => savePrefs({ ...prefs, view: 'grid' })}>
              <LayoutGridIcon className="size-4" />
            </Button>
            <Button type="button" variant={prefs.view === 'list' ? 'secondary' : 'ghost'} size="icon" className="size-7" aria-pressed={prefs.view === 'list'} aria-label="List" onClick={() => savePrefs({ ...prefs, view: 'list' })}>
              <ListIcon className="size-4" />
            </Button>
          </div>
          {prefs.view === 'grid' ? (
            <Select value={prefs.size} onValueChange={(v) => savePrefs({ ...prefs, size: v as GridSize })}>
              <SelectTrigger size="sm" className="w-[8rem]" aria-label="Tile size">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="large">Large tiles</SelectItem>
                <SelectItem value="small">Small tiles</SelectItem>
              </SelectContent>
            </Select>
          ) : null}
        </div>
      </DialogHeader>

      <div className="grid min-h-0 flex-1 md:grid-cols-[14rem_minmax(0,1fr)] lg:grid-cols-[14rem_minmax(0,1fr)_20rem]">
        <aside className="hidden min-h-0 flex-col border-r md:flex" aria-label="Folders">
          <div className="min-h-0 flex-1 overflow-y-auto p-2" role="tree" aria-label="Folders">
            <RootRow label="All media" isActive={folder === ''} tabbable={folder === ''} onClick={() => setFolder('')} />
            <DirectoryTree nodes={tree} activePath={folder} onNavigate={setFolder} />
          </div>
          {canManage ? (
            <div className="border-t p-2">
              {newFolder === null ? (
                <Button type="button" variant="ghost" size="sm" className="w-full justify-start" onClick={() => setNewFolder('')}>
                  <FolderPlusIcon className="size-4" />
                  New folder{folder ? ' here' : ''}
                </Button>
              ) : (
                <form className="flex gap-1" onSubmit={(e) => { e.preventDefault(); void submitFolder() }}>
                  <Input value={newFolder} onChange={(e) => setNewFolder(e.target.value)} placeholder="Folder name" aria-label="New folder name" className="h-8" autoFocus disabled={creating} />
                  <Button type="submit" size="sm" className="h-8" disabled={creating}>Add</Button>
                </form>
              )}
            </div>
          ) : null}
        </aside>

        <section className="flex min-h-0 flex-col" aria-label="Media">
          <div className="flex flex-wrap items-center justify-between gap-2 border-b px-4 py-2 text-xs text-muted-foreground">
            <span>
              {folder ? <span className="font-medium text-foreground">{folder}</span> : 'All media'}
              {' · '}
              {results.loading && items.length === 0 ? 'Loading…' : `${total} ${total === 1 ? 'item' : 'items'}`}
              {folder ? ' including subfolders' : ''}
            </span>
            <select
              aria-label="Folder"
              className="h-7 rounded-md border bg-background px-2 text-xs md:hidden"
              value={folder}
              onChange={(e) => setFolder(e.target.value)}
            >
              <option value="">All media</option>
              {(dirs.data?.allMediaDirectories ?? []).map((d) => (
                <option key={d.path} value={d.path}>{d.path}</option>
              ))}
            </select>
          </div>

          {uploads.items.length > 0 ? (
            <ul className="grid gap-1 border-b px-4 py-2" aria-label="Uploads">
              {uploads.items.map((u) => (
                <li key={u.key} className="flex items-center gap-3 text-xs">
                  <span className="min-w-0 flex-1 truncate">{u.name}</span>
                  {u.status === 'uploading' ? (
                    <span className="h-1.5 w-32 overflow-hidden rounded-full bg-muted" role="progressbar" aria-valuenow={Math.round(u.progress * 100)} aria-label={`Uploading ${u.name}`}>
                      <span className="block h-full bg-primary transition-[width]" style={{ width: `${Math.max(4, u.progress * 100)}%` }} />
                    </span>
                  ) : u.status === 'done' ? (
                    <span className="text-success">Added</span>
                  ) : (
                    <span className="text-destructive">{u.error}</span>
                  )}
                  {u.status !== 'uploading' ? (
                    <button type="button" aria-label={`Dismiss ${u.name}`} onClick={() => uploads.dismiss(u.key)} className="text-muted-foreground hover:text-foreground">
                      <XIcon className="size-3.5" />
                    </button>
                  ) : null}
                </li>
              ))}
            </ul>
          ) : null}

          <div className="min-h-0 flex-1 overflow-y-auto p-4">
            {items.length === 0 && !results.loading ? (
              <div className="grid h-full place-items-center">
                <div className="grid max-w-sm justify-items-center gap-2 text-center">
                  <UploadCloudIcon className="size-10 text-muted-foreground" />
                  <p className="text-sm font-medium">{filtered ? 'Nothing matches' : folder ? 'This folder is empty' : 'The library is empty'}</p>
                  <p className="text-sm text-muted-foreground">
                    {filtered ? 'Try other words, or clear the search.' : canManage ? 'Drop files here, paste one, or upload from your computer.' : 'Nothing has been uploaded yet.'}
                  </p>
                  {filtered ? (
                    <Button type="button" size="sm" variant="outline" onClick={() => setSearch('')}>Clear the search</Button>
                  ) : canManage ? (
                    <Button type="button" size="sm" onClick={() => fileInput.current?.click()}>
                      <UploadIcon className="size-4" />
                      Upload
                    </Button>
                  ) : null}
                </div>
              </div>
            ) : (
              <>
                <AssetGrid
                  assets={items}
                  view={prefs.view}
                  size={prefs.size}
                  selectedIds={new Set(picked)}
                  focusedId={focusedId}
                  pickable={pickable}
                  onFocus={setFocusedId}
                  onToggle={toggle}
                  onUse={(a) => finish(multiple ? [...new Set([...picked, a.id])] : [a.id])}
                />
                {hasMore ? (
                  <div ref={sentinel} className="flex justify-center py-4">
                    <Button type="button" variant="outline" size="sm" disabled={results.loading} onClick={() => void loadMore()}>
                      {results.loading ? 'Loading…' : `Load more (${items.length} of ${total})`}
                    </Button>
                  </div>
                ) : null}
              </>
            )}
          </div>
        </section>

        <aside className="hidden min-h-0 overflow-y-auto border-l p-4 lg:block" aria-label="Details">
          {focused ? (
            <AssetPanel key={focused.id} asset={focused} canManage={canManage} onChanged={(a) => remember([a])} />
          ) : (
            <p className="pt-10 text-center text-sm text-muted-foreground">Pick something to see its details.</p>
          )}
        </aside>
      </div>

      <div className="flex flex-wrap items-center justify-between gap-3 border-t px-5 py-3">
        <div className="flex min-w-0 items-center gap-2">
          {pickedAssets.slice(0, 6).map((a) => (
            <Thumb key={a.id} asset={a} className="size-9 shrink-0 rounded border" />
          ))}
          <span className="truncate text-sm text-muted-foreground">
            {pickedAssets.length === 0
              ? 'Nothing picked yet'
              : multiple
                ? `${pickedAssets.length} picked`
                : pickedAssets[0].title || pickedAssets[0].filename}
          </span>
          {multiple && pickedAssets.length > 0 ? (
            <Button type="button" variant="ghost" size="sm" onClick={() => setPicked([])}>Clear</Button>
          ) : null}
        </div>
        <div className="flex gap-2">
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>Cancel</Button>
          <Button type="button" disabled={pickedAssets.length === 0 || uploads.busy} onClick={() => finish(picked)}>
            {multiple ? `Use ${pickedAssets.length || ''} ${pickedAssets.length === 1 ? 'file' : 'files'}`.replace('  ', ' ') : accept === 'image' ? 'Use picture' : 'Use file'}
          </Button>
        </div>
      </div>

      <input
        ref={fileInput}
        type="file"
        multiple
        accept={accept === 'image' ? 'image/*' : undefined}
        className="hidden"
        aria-label="Upload files"
        onChange={(e) => {
          addFiles(Array.from(e.target.files ?? []))
          e.target.value = ''
        }}
      />

      {dragging ? (
        <div className="pointer-events-none absolute inset-2 z-10 grid place-items-center rounded-lg border-2 border-dashed border-primary bg-background/85">
          <div className="grid justify-items-center gap-2 text-center">
            <UploadCloudIcon className="size-10 text-primary" />
            <p className="text-sm font-medium">Drop to upload to {folder || uploadPath}</p>
          </div>
        </div>
      ) : null}
    </div>
  )
}
