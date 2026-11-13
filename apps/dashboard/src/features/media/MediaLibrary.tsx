import { useCallback, useRef, useState, type ClipboardEvent, type DragEvent } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import {
  ChevronRightIcon,
  FolderIcon,
  FolderPlusIcon,
  LayoutGridIcon,
  ListIcon,
  MoreHorizontalIcon,
  PencilIcon,
  SearchIcon,
  Trash2Icon,
  UploadCloudIcon,
  UploadIcon,
  XIcon,
} from 'lucide-react'
import { DetailPageLayout } from '@/components/layout/detail-page'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import type { MediaAssetSort, MediaMimeGroup } from '@/graphql/schema.types'
import { formatNumber } from '@/lib/format'
import { useUnsavedChangesGuard } from '@/lib/unsaved-changes'
import { cn } from '@/lib/utils'
import { usePermissions } from '@/shared/auth/usePermissions'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'
import { useConfirmDialog } from '@/shared/dialogs/useConfirmDialog'
import { useDebounced } from '@/shared/hooks/useDebounced'
import { buildBreadcrumbs, buildDirectoryTree } from './directory-paths'
import { FolderDialog } from './library/FolderDialog'
import { FolderRail } from './library/FolderRail'
import { Gallery } from './library/Gallery'
import { Inspector } from './library/Inspector'
import { ROW_MAX, ROW_MIN, SORTS, readPrefs, savePrefs, type LibraryPrefs } from './library/prefs'
import { SelectionBar } from './library/SelectionBar'
import { Untracked } from './library/Untracked'
import {
  AllMediaDirectoriesDocument,
  CreateMediaDirectoryDocument,
  DeleteMediaAssetDocument,
  DeleteMediaDirectoryDocument,
  MediaCountsDocument,
  MediaDirectoryDocument,
  MoveMediaAssetDocument,
  RenameMediaDirectoryDocument,
  type MediaAssetFieldsFragment as MediaAsset,
} from './MediaLibrary.gql'
import { AssetGrid } from './picker/AssetGrid'
import { useUploadQueue } from './picker/useUploadQueue'
import { useMediaSearch } from './useMediaSearch'

// Exactly what the backend accepts (internal/media/allowed.go plus the sanitized SVG route). The
// old image/* let the picker offer formats the server then refused, so you found out after choosing
const UPLOAD_ACCEPT = 'image/jpeg,image/png,image/webp,image/gif,image/heic,image/heif,image/svg+xml,application/pdf'

type TypeFilter = MediaMimeGroup | 'ALL'
const TYPES: { value: TypeFilter; label: string; count: 'all' | 'images' | 'documents' }[] = [
  { value: 'ALL', label: 'Everything', count: 'all' },
  { value: 'IMAGE', label: 'Images', count: 'images' },
  { value: 'DOCUMENT', label: 'Documents', count: 'documents' },
]

const parentOf = (path: string) => (path.includes('/') ? path.split('/').slice(0, -1).join('/') : '')

// The library as a wall of its pictures, the folders docked on one side and the picked file on the other
export function MediaLibrary() {
  const [confirm, confirmDialog] = useConfirmDialog()
  const [searchParams, setSearchParams] = useSearchParams()
  const path = searchParams.get('path') ?? ''
  const search = searchParams.get('q') ?? ''
  const query = useDebounced(search.trim(), 250)
  const { has } = usePermissions()
  const canManage = has('media.manage')

  const [type, setType] = useState<TypeFilter>('ALL')
  const [prefs, setPrefsState] = useState(readPrefs)
  const setPrefs = (next: LibraryPrefs) => {
    setPrefsState(next)
    savePrefs(next)
  }
  const [picked, setPicked] = useState<string[]>([])
  const [focusedId, setFocusedId] = useState<string | null>(null)
  const [anchorId, setAnchorId] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [dragging, setDragging] = useState(false)
  const [folderDialog, setFolderDialog] = useState<'new' | 'rename' | null>(null)
  const [working, setWorking] = useState(false)
  const fileInput = useRef<HTMLInputElement>(null)

  // Typed words in the details panel that aren't saved yet. Whatever would take the panel off its file asks first
  const [panelDirty, setPanelDirty] = useState(false)
  const [afterDiscard, setAfterDiscard] = useState<(() => void) | null>(null)
  useUnsavedChangesGuard(panelDirty, 'Media asset')

  // Another folder or search lets several picks go, but one open file stays open, so nothing typed for it is lost
  const wall = `${path}\n${query}\n${type}`
  const [shownWall, setShownWall] = useState(wall)
  if (shownWall !== wall) {
    setShownWall(wall)
    setPicked((p) => (p.length === 1 ? p : []))
    setAnchorId(null)
  }

  const dirs = useQuery(AllMediaDirectoriesDocument)
  const folders = (dirs.data?.allMediaDirectories ?? []).map((d) => d.path)
  const directory = useQuery(MediaDirectoryDocument, { variables: { path } })
  const counts = useQuery(MediaCountsDocument, { variables: { query: query || null, path: path || null }, fetchPolicy: 'cache-and-network' })
  const results = useMediaSearch({ query, path, type, sort: prefs.sort })
  const { items, total, loading, hasMore, loadMore, sentinel } = results

  const byId = new Map(items.map((a) => [a.id, a]))
  const pickedAssets = picked.map((id) => byId.get(id)).filter((a): a is MediaAsset => !!a)
  const [seen, setSeen] = useState<MediaAsset | null>(null)
  const onWall = picked.length === 1 ? byId.get(picked[0]!) : undefined
  if (onWall && onWall !== seen) setSeen(onWall)
  const open = picked.length === 1 ? (onWall ?? (seen?.id === picked[0] ? seen : null)) : null

  const { refetch: refetchResults } = results
  const { refetch: refetchCounts } = counts
  const { refetch: refetchDirectory } = directory
  const { refetch: refetchDirs } = dirs
  const refresh = useCallback(async () => {
    await Promise.all([refetchResults(), refetchCounts(), refetchDirectory(), refetchDirs()].map((p) => p.catch(() => {})))
  }, [refetchResults, refetchCounts, refetchDirectory, refetchDirs])

  function navigateTo(next: string) {
    const params = new URLSearchParams(searchParams)
    if (next) params.set('path', next)
    else params.delete('path')
    setSearchParams(params)
  }
  function setSearch(next: string) {
    const params = new URLSearchParams(searchParams)
    if (next) params.set('q', next)
    else params.delete('q')
    setSearchParams(params, { replace: true })
  }

  // Picks change through here, so edits waiting in the panel aren't dropped without a word
  function pick(next: string[]) {
    const keepsPanel = next.length === 1 && next[0] === open?.id
    if (panelDirty && !keepsPanel) setAfterDiscard(() => () => setPicked(next))
    else setPicked(next)
  }

  function toggle(a: MediaAsset, { range, additive }: { range: boolean; additive: boolean }) {
    setFocusedId(a.id)
    if (range && anchorId) {
      const from = items.findIndex((x) => x.id === anchorId)
      const to = items.findIndex((x) => x.id === a.id)
      if (from >= 0 && to >= 0) {
        const span = items.slice(Math.min(from, to), Math.max(from, to) + 1).map((x) => x.id)
        return pick([...new Set([...picked, ...span])])
      }
    }
    setAnchorId(a.id)
    if (additive) return pick(picked.includes(a.id) ? picked.filter((id) => id !== a.id) : [...picked, a.id])
    pick(picked.length === 1 && picked[0] === a.id ? [] : [a.id])
  }

  const uploads = useUploadQueue(
    path,
    useCallback(
      async (ids: string[]) => {
        await refresh()
        if (ids.length > 0) setPicked([ids[ids.length - 1]!])
      },
      [refresh],
    ),
  )
  function addFiles(files: File[]) {
    if (canManage && files.length > 0) void uploads.add(files, () => null)
  }
  function onDrop(e: DragEvent) {
    e.preventDefault()
    setDragging(false)
    addFiles(Array.from(e.dataTransfer.files))
  }
  function onPaste(e: ClipboardEvent) {
    const files = Array.from(e.clipboardData.files)
    if (files.length > 0) {
      e.preventDefault()
      addFiles(files)
    }
  }

  const [createDirectory, { loading: creating }] = useMutation(CreateMediaDirectoryDocument)
  const [renameDirectory, { loading: renaming }] = useMutation(RenameMediaDirectoryDocument)
  const [deleteDirectory] = useMutation(DeleteMediaDirectoryDocument)
  const [moveAsset] = useMutation(MoveMediaAssetDocument)
  const [deleteAsset] = useMutation(DeleteMediaAssetDocument)

  async function run(what: string, fn: () => Promise<void>) {
    setError(null)
    try {
      await fn()
    } catch (e) {
      setError(e instanceof Error ? e.message : `${what} failed.`)
    }
  }

  const onFolderName = (name: string) =>
    run(folderDialog === 'rename' ? 'Renaming the folder' : 'Making the folder', async () => {
      if (folderDialog === 'rename') {
        const res = await renameDirectory({ variables: { path, newName: name } })
        setFolderDialog(null)
        await refetchDirs()
        navigateTo(res.data?.renameMediaDirectory.path ?? parentOf(path))
        return
      }
      await createDirectory({ variables: { path, name } })
      setFolderDialog(null)
      await refresh()
    })

  const askDeleteFolder = (recursive: boolean) =>
    confirm({
      title: recursive ? `Delete ${path} and everything in it?` : `Delete ${path}?`,
      description: recursive ? 'Every file and folder inside goes with it, from storage as well.' : 'It has to be empty.',
      confirmLabel: 'Delete folder',
      destructive: true,
      onConfirm: () =>
        run('Deleting the folder', async () => {
          await deleteDirectory({ variables: { path, recursive } })
          await refetchDirs()
          navigateTo(parentOf(path))
        }),
    })

  // One at a time, so a failure names how many made it and the rest stay picked
  async function forEachPicked(what: string, act: (a: MediaAsset) => Promise<unknown>) {
    setWorking(true)
    setError(null)
    const failed: string[] = []
    let reason = ''
    for (const a of pickedAssets) {
      try {
        await act(a)
      } catch (e) {
        failed.push(a.id)
        reason = e instanceof Error ? e.message : ''
      }
    }
    await refresh()
    setPicked(failed)
    setWorking(false)
    if (failed.length > 0) setError(`${failed.length} of ${pickedAssets.length} couldn't be ${what}${reason ? `: ${reason}` : '.'}`)
  }

  const tree = buildDirectoryTree(folders)
  const breadcrumbs = buildBreadcrumbs(path)
  const subfolders = query ? [] : (directory.data?.mediaDirectory.directories ?? [])
  const untracked = query ? [] : (directory.data?.mediaDirectory.untrackedFiles ?? [])
  const filtered = !!query || type !== 'ALL'
  const loadError = results.error ?? directory.error

  return (
    <DetailPageLayout
      className="relative @container/media"
      leftSidebar={<FolderRail tree={tree} path={path} canManage={canManage} onNavigate={navigateTo} onNewFolder={() => setFolderDialog('new')} />}
      rightSidebar={
        open ? (
          <Inspector
            asset={open}
            canManage={canManage}
            folders={folders}
            onChanged={(a) => {
              if (a.directoryPath !== open.directoryPath || a.filename !== open.filename) void refresh()
            }}
            onDeleted={() => {
              setPicked([])
              void refresh()
            }}
            onClose={() => pick([])}
            onDirty={setPanelDirty}
            onError={setError}
          />
        ) : null
      }
    >
      <div
        className="relative flex min-h-full flex-col"
        onPaste={onPaste}
        onKeyDown={(e) => {
          if (e.key === 'Escape' && picked.length > 0 && !(e.target instanceof HTMLInputElement)) pick([])
        }}
        onDragEnter={(e) => {
          if (canManage && e.dataTransfer.types.includes('Files')) {
            e.preventDefault()
            setDragging(true)
          }
        }}
        onDragOver={(e) => canManage && e.dataTransfer.types.includes('Files') && e.preventDefault()}
        onDragLeave={(e) => {
          if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setDragging(false)
        }}
        onDrop={onDrop}
      >
        <div className="sticky top-0 z-10 border-b border-foreground/10 bg-background/85 backdrop-blur-md">
          <div className="flex min-h-[49px] flex-wrap items-center justify-between gap-3 px-4 py-2">
            <nav aria-label="Folder" className="flex min-w-0 flex-wrap items-center gap-0.5 text-sm">
              {breadcrumbs.map((crumb, i) => (
                <span key={crumb.path || 'root'} className="inline-flex items-center gap-0.5">
                  {i > 0 ? <span className="text-muted-foreground/60">/</span> : null}
                  <button
                    type="button"
                    aria-current={i === breadcrumbs.length - 1 ? 'page' : undefined}
                    onClick={() => navigateTo(crumb.path)}
                    className={cn('rounded-md px-1.5 py-0.5 transition-colors hover:bg-muted', i === breadcrumbs.length - 1 ? 'font-medium text-foreground' : 'text-muted-foreground hover:text-foreground')}
                  >
                    {crumb.name}
                  </button>
                </span>
              ))}
            </nav>
            {canManage ? (
              <div className="flex shrink-0 items-center gap-2">
                {path ? (
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button variant="outline" size="sm">
                        <MoreHorizontalIcon className="size-4" />
                        Folder
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem onSelect={() => setFolderDialog('rename')}>
                        <PencilIcon className="size-4" />
                        Rename
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem variant="destructive" onSelect={() => askDeleteFolder(false)}>
                        <Trash2Icon className="size-4" />
                        Delete if empty
                      </DropdownMenuItem>
                      <DropdownMenuItem variant="destructive" onSelect={() => askDeleteFolder(true)}>
                        <Trash2Icon className="size-4" />
                        Delete with everything in it
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                ) : null}
                <Button variant="outline" size="sm" className="@min-[56rem]/media:hidden" onClick={() => setFolderDialog('new')}>
                  <FolderPlusIcon className="size-4" />
                  New folder
                </Button>
                <Button size="sm" onClick={() => fileInput.current?.click()} disabled={uploads.busy}>
                  <UploadIcon className="size-4" />
                  Upload
                </Button>
              </div>
            ) : null}
          </div>
        </div>

        <div className="@container grid gap-5 px-4 pt-4 pb-6">
          <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
            <div role="radiogroup" aria-label="Show" className="flex flex-wrap gap-1">
              {TYPES.map((t) => (
                <button
                  key={t.value}
                  type="button"
                  role="radio"
                  aria-checked={type === t.value}
                  onClick={() => setType(t.value)}
                  className={cn(
                    'flex items-center gap-1.5 rounded-full px-3 py-1.5 text-sm outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring',
                    type === t.value ? 'bg-foreground text-background' : 'text-muted-foreground hover:bg-muted hover:text-foreground',
                  )}
                >
                  {t.label}
                  {counts.data ? <span className="tabular-nums opacity-70">{formatNumber(counts.data[t.count].totalCount)}</span> : null}
                </button>
              ))}
            </div>
            <div className="relative min-w-48 flex-1 @3xl:max-w-sm">
              <SearchIcon aria-hidden className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                type="search"
                aria-label="Search media"
                placeholder="Search names, titles, alt text and captions"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className="h-9 rounded-full pr-8 pl-9"
              />
              {search ? (
                <button type="button" aria-label="Clear search" onClick={() => setSearch('')} className="absolute top-1/2 right-2.5 -translate-y-1/2 text-muted-foreground hover:text-foreground">
                  <XIcon className="size-4" />
                </button>
              ) : null}
            </div>
            <div className="ml-auto flex flex-wrap items-center gap-2">
              <Select value={prefs.sort} onValueChange={(v) => setPrefs({ ...prefs, sort: v as MediaAssetSort })}>
                <SelectTrigger size="sm" className="w-[9.5rem] rounded-full" aria-label="Sort">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {SORTS.map((s) => (
                    <SelectItem key={s.value} value={s.value}>
                      {s.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {prefs.view === 'gallery' ? (
                <input
                  type="range"
                  aria-label="Picture size"
                  min={ROW_MIN}
                  max={ROW_MAX}
                  step={10}
                  value={prefs.rowHeight}
                  onChange={(e) => setPrefs({ ...prefs, rowHeight: Number(e.target.value) })}
                  className="h-1 w-24 cursor-pointer accent-foreground"
                />
              ) : null}
              <div role="group" aria-label="View" className="flex items-center rounded-full border p-0.5">
                <Button type="button" variant={prefs.view === 'gallery' ? 'secondary' : 'ghost'} size="icon" className="size-7 rounded-full" aria-pressed={prefs.view === 'gallery'} aria-label="Gallery" onClick={() => setPrefs({ ...prefs, view: 'gallery' })}>
                  <LayoutGridIcon className="size-4" />
                </Button>
                <Button type="button" variant={prefs.view === 'list' ? 'secondary' : 'ghost'} size="icon" className="size-7 rounded-full" aria-pressed={prefs.view === 'list'} aria-label="List" onClick={() => setPrefs({ ...prefs, view: 'list' })}>
                  <ListIcon className="size-4" />
                </Button>
              </div>
            </div>
          </div>

          {uploads.items.length > 0 ? (
            <ul aria-label="Uploads" className="grid gap-1.5 rounded-lg border bg-muted/40 px-3 py-2.5">
              {uploads.items.map((u) => (
                <li key={u.key} className="flex items-center gap-3 text-xs">
                  <span className="min-w-0 flex-1 truncate">{u.name}</span>
                  {u.status === 'uploading' ? (
                    <span className="h-1.5 w-40 overflow-hidden rounded-full bg-muted" role="progressbar" aria-valuenow={Math.round(u.progress * 100)} aria-label={`Uploading ${u.name}`}>
                      <span className="block h-full bg-foreground transition-[width]" style={{ width: `${Math.max(4, u.progress * 100)}%` }} />
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

          {error || loadError ? (
            <Alert variant="destructive">
              <AlertTitle>{error ? "That didn't go through" : "The library couldn't be loaded"}</AlertTitle>
              <AlertDescription>{error ?? loadError?.message}</AlertDescription>
            </Alert>
          ) : null}

          {subfolders.length > 0 ? (
            <ul aria-label="Folders here" className="flex flex-wrap gap-2">
              {subfolders.map((d) => (
                <li key={d.path}>
                  <button
                    type="button"
                    onClick={() => navigateTo(d.path)}
                    className="group flex items-center gap-2 rounded-lg border px-3 py-2 text-sm outline-none transition-colors hover:border-foreground/40 hover:bg-muted/50 focus-visible:ring-2 focus-visible:ring-ring"
                  >
                    <FolderIcon className="size-4 text-muted-foreground" />
                    {d.name}
                    <ChevronRightIcon className="size-3.5 text-muted-foreground transition-transform group-hover:translate-x-0.5" />
                  </button>
                </li>
              ))}
            </ul>
          ) : null}

          <section aria-label="Files" className="grid gap-3">
            <p className="text-xs text-muted-foreground">
              {loading && items.length === 0
                ? 'Loading…'
                : `${formatNumber(total)} ${total === 1 ? 'file' : 'files'}${query ? ` for "${query}"` : ''} in ${path || 'the library'}${path ? ', its folders included' : ''}`}
            </p>
            {items.length === 0 && !loading ? (
              <div className="grid min-h-[45vh] place-items-center rounded-xl border border-dashed">
                <div className="grid max-w-sm justify-items-center gap-2 p-8 text-center">
                  <UploadCloudIcon className="size-10 text-muted-foreground" />
                  <p className="text-base font-medium">{filtered ? 'Nothing matches' : path ? 'This folder is empty' : 'The library is empty'}</p>
                  <p className="text-sm text-muted-foreground">
                    {filtered ? 'Try other words, or show everything again.' : canManage ? 'Drop files here, paste a picture, or upload from your computer.' : 'Nothing has been uploaded yet.'}
                  </p>
                  {filtered ? (
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={() => {
                        setType('ALL')
                        setSearch('')
                      }}
                    >
                      Show everything
                    </Button>
                  ) : canManage ? (
                    <Button type="button" size="sm" onClick={() => fileInput.current?.click()}>
                      <UploadIcon className="size-4" />
                      Upload
                    </Button>
                  ) : null}
                </div>
              </div>
            ) : prefs.view === 'gallery' ? (
              <Gallery assets={items} rowHeight={prefs.rowHeight} selectedIds={new Set(picked)} focusedId={focusedId} onFocus={setFocusedId} onToggle={toggle} onOpen={(a) => pick([a.id])} />
            ) : (
              <AssetGrid
                assets={items}
                view="list"
                size="large"
                selectedIds={new Set(picked)}
                focusedId={focusedId}
                pickable={() => null}
                onFocus={setFocusedId}
                onToggle={toggle}
                onUse={(a) => pick([a.id])}
              />
            )}
            {hasMore ? (
              <div ref={sentinel} className="flex justify-center py-4">
                <Button type="button" variant="outline" size="sm" disabled={loading} onClick={() => void loadMore()}>
                  {loading ? 'Loading…' : `Load more (${formatNumber(items.length)} of ${formatNumber(total)})`}
                </Button>
              </div>
            ) : null}
          </section>

          <Untracked files={untracked} />
        </div>

        {pickedAssets.length > 1 && canManage ? (
          <SelectionBar
            count={pickedAssets.length}
            folders={folders}
            busy={working}
            onMove={(folder) => void forEachPicked('moved', (a) => moveAsset({ variables: { id: a.id, targetPath: folder, filename: a.filename } }))}
            onDelete={() =>
              confirm({
                title: `Delete ${pickedAssets.length} files?`,
                description: 'They are removed from storage as well, so anything still using them will show a broken picture.',
                confirmLabel: 'Delete files',
                destructive: true,
                onConfirm: () => forEachPicked('deleted', (a) => deleteAsset({ variables: { id: a.id, deleteFromStorage: true } })),
              })
            }
            onClear={() => pick([])}
          />
        ) : null}

        {dragging ? (
          <div className="pointer-events-none absolute inset-0 z-20 p-3">
            <div className="sticky top-3 grid h-[min(70vh,32rem)] place-items-center rounded-2xl border-2 border-dashed border-foreground/60 bg-background/90 backdrop-blur-sm">
              <div className="grid justify-items-center gap-2 text-center">
                <UploadCloudIcon className="size-10" />
                <p className="text-2xl font-light tracking-tight">Drop to add to {path || 'the library'}</p>
              </div>
            </div>
          </div>
        ) : null}

        <input
          ref={fileInput}
          type="file"
          multiple
          accept={UPLOAD_ACCEPT}
          className="hidden"
          aria-label="Upload files"
          onChange={(e) => {
            addFiles(Array.from(e.target.files ?? []))
            e.target.value = ''
          }}
        />
      </div>

      <FolderDialog
        open={folderDialog !== null}
        title={folderDialog === 'rename' ? 'Rename the folder' : 'New folder'}
        description={folderDialog === 'rename' ? 'The files inside move with it.' : `It is made inside ${path || 'the library'}.`}
        initial={folderDialog === 'rename' ? (path.split('/').at(-1) ?? '') : ''}
        submitLabel={folderDialog === 'rename' ? 'Rename' : 'Create folder'}
        busy={creating || renaming}
        onOpenChange={(o) => !o && setFolderDialog(null)}
        onSubmit={(name) => void onFolderName(name)}
      />
      <ConfirmDialog
        open={afterDiscard !== null}
        onOpenChange={(o) => !o && setAfterDiscard(null)}
        title="Discard changes to this file?"
        description="The words you typed for it haven't been saved."
        confirmLabel="Discard"
        destructive
        onConfirm={() => {
          setPanelDirty(false)
          afterDiscard?.()
          setAfterDiscard(null)
        }}
      />
      {confirmDialog}
    </DetailPageLayout>
  )
}
