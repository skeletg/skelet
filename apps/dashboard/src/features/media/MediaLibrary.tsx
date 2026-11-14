import { Suspense, lazy, useCallback, useEffect, useRef, useState, type ClipboardEvent, type DragEvent } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { ChevronRightIcon, FolderIcon, UploadCloudIcon } from 'lucide-react'
import { DetailPageLayout } from '@/components/layout/detail-page'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import type { MediaVisibility } from '@/graphql/schema.types'
import { formatNumber } from '@/lib/format'
import { useUnsavedChangesGuard } from '@/lib/unsaved-changes'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'
import { useConfirmDialog } from '@/shared/dialogs/useConfirmDialog'
import { useDebounced } from '@/shared/hooks/useDebounced'
import { buildDirectoryTree } from './directory-paths'
import { EmptyWall } from './library/EmptyWall'
import { Filters, type TypeFilter } from './library/Filters'
import { FolderDialog } from './library/FolderDialog'
import { FolderRail } from './library/FolderRail'
import { Gallery } from './library/Gallery'
import { Inspector } from './library/Inspector'
import { PathBar } from './library/PathBar'
import { readPrefs, savePrefs, type LibraryPrefs } from './library/prefs'
import { SelectionBar } from './library/SelectionBar'
import { Untracked } from './library/Untracked'
import { UploadList } from './library/UploadList'
import {
  AllMediaDirectoriesDocument,
  CreateMediaDirectoryDocument,
  DeleteMediaAssetDocument,
  DeleteMediaDirectoryDocument,
  MediaCountsDocument,
  MediaDirectoryDocument,
  MediaUploadTypesDocument,
  MoveMediaAssetDocument,
  RenameMediaDirectoryDocument,
  type MediaAssetFieldsFragment as MediaAsset,
} from './MediaLibrary.gql'
import { AssetGrid } from './picker/AssetGrid'
import { useUploadQueue } from './picker/useUploadQueue'
import { useMediaAccess } from './useMediaAccess'
import { useMediaSearch } from './useMediaSearch'

const PhotoEditor = lazy(() => import('./editor/PhotoEditor'))

const parentOf = (path: string) => (path.includes('/') ? path.split('/').slice(0, -1).join('/') : '')
// A folder's wall shows what is in it and under it
const under = (folder: string, path: string) => path === '' || folder === path || folder.startsWith(`${path}/`)

// The library as a wall of its pictures, the folders docked on one side and the picked file on the other
export function MediaLibrary() {
  const [confirm, confirmDialog] = useConfirmDialog()
  const [searchParams, setSearchParams] = useSearchParams()
  const path = searchParams.get('path') ?? ''
  const search = searchParams.get('q') ?? ''
  const query = useDebounced(search.trim(), 250)
  const access = useMediaAccess()
  const { hasPrivate } = access
  // The halves keep separate folders and files, so everything below asks about one. The public half asks exactly what it always did
  const area: MediaVisibility = hasPrivate && searchParams.get('area') === 'private' ? 'PRIVATE' : 'PUBLIC'
  // An address that asks for the private half waits to learn whether it may have it, rather than show the public one first
  const waiting = searchParams.get('area') === 'private' && !access.settled
  const canManage = access.canManage(area)
  const inArea = area === 'PRIVATE' ? { visibility: area } : {}

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
  const [editing, setEditing] = useState<MediaAsset | null>(null)
  const fileInput = useRef<HTMLInputElement>(null)

  // Typed words in the details panel that aren't saved yet. Whatever would take the panel off its file asks first
  const [panelDirty, setPanelDirty] = useState(false)
  const [afterDiscard, setAfterDiscard] = useState<(() => void) | null>(null)
  useUnsavedChangesGuard(panelDirty, 'Media asset')
  const dirtyNow = useRef(false)
  useEffect(() => {
    dirtyNow.current = panelDirty
  }, [panelDirty])

  // Another folder or search lets several picks go, but one open file stays open, so nothing typed for it is lost
  const wall = `${area}\n${path}\n${query}\n${type}`
  const [shownWall, setShownWall] = useState(wall)
  if (shownWall !== wall) {
    setShownWall(wall)
    setPicked((p) => (p.length === 1 ? p : []))
    setAnchorId(null)
  }

  const dirs = useQuery(AllMediaDirectoriesDocument, { variables: inArea, skip: waiting })
  const folders = (dirs.data?.allMediaDirectories ?? []).map((d) => d.path)
  const directory = useQuery(MediaDirectoryDocument, { variables: { path, ...inArea }, skip: waiting })
  const uploadTypes = useQuery(MediaUploadTypesDocument, { skip: !canManage }).data?.mediaUploadTypes
  const counts = useQuery(MediaCountsDocument, { variables: { query: query || null, path: path || null, ...inArea }, fetchPolicy: 'cache-and-network', skip: waiting })
  const results = useMediaSearch({ query, path, type, sort: prefs.sort, visibility: area, skip: waiting })
  const { items, total, loading, hasMore, moreFailed, loadMore, sentinel, drop } = results

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
  // The other half is another place: its own folders, nothing picked, and no search carried over
  function switchArea(next: MediaVisibility) {
    const go = () => {
      setPicked([])
      setSearchParams(next === 'PRIVATE' ? { area: 'private' } : {})
    }
    if (panelDirty) setAfterDiscard(() => go)
    else go()
  }
  const onArea = hasPrivate ? switchArea : undefined
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
        // an upload that lands while words are being typed for another file leaves that file open
        if (ids.length > 0 && !dirtyNow.current) setPicked([ids[ids.length - 1]!])
      },
      [refresh],
    ),
    area,
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
        const res = await renameDirectory({ variables: { path, newName: name, ...inArea } })
        setFolderDialog(null)
        await refetchDirs()
        navigateTo(res.data?.renameMediaDirectory.path ?? parentOf(path))
        return
      }
      await createDirectory({ variables: { path, name, ...inArea } })
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
          await deleteDirectory({ variables: { path, recursive, ...inArea } })
          await refetchDirs()
          navigateTo(parentOf(path))
        }),
    })

  // Files that left this wall come off it where they stand, and the counts catch up
  function leave(ids: string[]) {
    if (ids.length === 0) return
    drop(ids)
    void refetchCounts().catch(() => {})
  }

  // One at a time, so a failure names how many made it and the rest stay picked. gone says whether a file left the wall
  async function forEachPicked(what: string, act: (a: MediaAsset) => Promise<unknown>, gone: boolean) {
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
    if (gone) leave(pickedAssets.filter((a) => !failed.includes(a.id)).map((a) => a.id))
    setPicked(failed)
    setWorking(false)
    if (failed.length > 0) setError(`${failed.length} of ${pickedAssets.length} couldn't be ${what}${reason ? `: ${reason}` : '.'}`)
  }

  const tree = buildDirectoryTree(folders)
  const subfolders = query ? [] : (directory.data?.mediaDirectory.directories ?? [])
  const untracked = query ? [] : (directory.data?.mediaDirectory.untrackedFiles ?? [])
  const filtered = !!query || type !== 'ALL'
  const loadError = results.error ?? directory.error ?? dirs.error
  const openFilePicker = () => fileInput.current?.click()

  return (
    <DetailPageLayout
      className="relative @container/media"
      leftSidebar={<FolderRail tree={tree} path={path} canManage={canManage} area={area} onArea={onArea} onNavigate={navigateTo} onNewFolder={() => setFolderDialog('new')} />}
      rightSidebar={
        open ? (
          <Inspector
            asset={open}
            canManage={canManage}
            folders={folders}
            canCross={access.canCross}
            dirty={panelDirty}
            onLeft={(a) => {
              setPicked([])
              leave([a.id])
            }}
            onChanged={(a) => {
              setSeen(a)
              if (!under(a.directoryPath, path)) leave([a.id])
            }}
            onDeleted={(a) => {
              setPicked([])
              leave([a.id])
            }}
            onEdit={() => setEditing(open)}
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
        <PathBar
          path={path}
          canManage={canManage}
          uploading={uploads.busy}
          area={area}
          onArea={onArea}
          onNavigate={navigateTo}
          onRename={() => setFolderDialog('rename')}
          onDelete={askDeleteFolder}
          onNewFolder={() => setFolderDialog('new')}
          onUpload={openFilePicker}
        />

        <div className="@container grid gap-5 px-4 pt-4 pb-6">
          <Filters type={type} counts={counts.data} search={search} prefs={prefs} onType={setType} onSearch={setSearch} onPrefs={setPrefs} />
          <UploadList items={uploads.items} onDismiss={uploads.dismiss} onClear={uploads.clearFinished} />

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
                : `${formatNumber(total)} ${area === 'PRIVATE' ? 'private ' : ''}${total === 1 ? 'file' : 'files'}${query ? ` for "${query}"` : ''} in ${path || 'the library'}${path ? ', its folders included' : ''}`}
            </p>
            {items.length === 0 && !loading ? (
              <EmptyWall
                filtered={filtered}
                path={path}
                canManage={canManage}
                onShowAll={() => {
                  setType('ALL')
                  setSearch('')
                }}
                onUpload={openFilePicker}
              />
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
                  {loading ? 'Loading…' : moreFailed ? "The rest couldn't be loaded. Try again" : `Load more (${formatNumber(items.length)} of ${formatNumber(total)})`}
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
            onMove={(folder) => void forEachPicked('moved', (a) => moveAsset({ variables: { id: a.id, targetPath: folder, filename: a.filename } }), !under(folder, path))}
            onDelete={() =>
              confirm({
                title: `Delete ${pickedAssets.length} files?`,
                description: 'They are removed from storage as well, so anything still using them will show a broken picture.',
                confirmLabel: 'Delete files',
                destructive: true,
                onConfirm: () => forEachPicked('deleted', (a) => deleteAsset({ variables: { id: a.id, deleteFromStorage: true } }), true),
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
                <p className="text-2xl font-light tracking-tight">
                  Drop to add to {path || 'the library'}
                  {area === 'PRIVATE' ? ', privately' : ''}
                </p>
              </div>
            </div>
          </div>
        ) : null}

        <input
          ref={fileInput}
          type="file"
          multiple
          accept={uploadTypes?.join(',')}
          className="hidden"
          aria-label="Upload files"
          onChange={(e) => {
            addFiles(Array.from(e.target.files ?? []))
            e.target.value = ''
          }}
        />
      </div>

      {editing ? (
        <Suspense fallback={null}>
          <PhotoEditor
            asset={editing}
            onClose={() => setEditing(null)}
            onSaved={async (id) => {
              setEditing(null)
              await refresh()
              pick([id])
            }}
          />
        </Suspense>
      ) : null}
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
