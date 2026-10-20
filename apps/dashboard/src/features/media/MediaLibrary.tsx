import { ADMIN_MEDIA_SVG_ENDPOINT } from '@/lib/api-endpoint'
import { getAuthStoredState } from '@/shared/auth/auth'
import { useEffect, useMemo, useRef, useState, type ChangeEvent, type FormEvent, type MouseEvent as ReactMouseEvent } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import {
  AlertTriangleIcon,
  ChevronDownIcon,
  ChevronRightIcon,
  ExternalLinkIcon,
  FileTextIcon,
  FolderIcon,
  FolderOpenIcon,
  MoreHorizontalIcon,
  PencilIcon,
  PlusIcon,
  SearchIcon,
  SparklesIcon,
  Trash2Icon,
  UploadIcon,
} from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Sheet,
  SheetContent,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'
import { usePermissions } from '@/shared/auth/usePermissions'
import {
  AllMediaDirectoriesDocument,
  ConfirmMediaAssetUploadDocument,
  CreateMediaDirectoryDocument,
  DeleteMediaAssetDocument,
  DeleteMediaDirectoryDocument,
  GenerateMediaAltTextDocument,
  MediaAiStatusDocument,
  MediaDirectoryDocument,
  MoveMediaAssetDocument,
  RenameMediaDirectoryDocument,
  RequestMediaAssetUploadDocument,
  SearchMediaAssetsDocument,
  SetMediaFocalPointDocument,
  UpdateMediaAssetDocument,
  type AllMediaDirectoriesQuery,
  type ConfirmMediaAssetUploadMutation,
  type ConfirmMediaAssetUploadMutationVariables,
  type CreateMediaDirectoryMutation,
  type CreateMediaDirectoryMutationVariables,
  type DeleteMediaAssetMutation,
  type DeleteMediaAssetMutationVariables,
  type DeleteMediaDirectoryMutation,
  type DeleteMediaDirectoryMutationVariables,
  type GenerateMediaAltTextMutation,
  type GenerateMediaAltTextMutationVariables,
  type MediaAiStatusQuery,
  type MediaAssetFieldsFragment as MediaAsset,
  type MediaDirectoryQuery,
  type MediaDirectoryQueryVariables,
  type MoveMediaAssetMutation,
  type MoveMediaAssetMutationVariables,
  type RenameMediaDirectoryMutation,
  type RenameMediaDirectoryMutationVariables,
  type RequestMediaAssetUploadMutation,
  type RequestMediaAssetUploadMutationVariables,
  type SearchMediaAssetsQuery,
  type SearchMediaAssetsQueryVariables,
  type SetMediaFocalPointMutation,
  type SetMediaFocalPointMutationVariables,
  type UpdateMediaAssetMutation,
  type UpdateMediaAssetMutationVariables,
} from './MediaLibrary.gql'
import { readImageDimensions } from '@/lib/image'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'
import { useUnsavedChangesGuard } from '@/lib/unsaved-changes'
import { onTreeItemKeyDown } from '@/lib/tree-keys'

type UntrackedMediaFile = MediaDirectoryQuery['mediaDirectory']['untrackedFiles'][number]

// Exactly what the backend accepts (internal/media/allowed.go plus the sanitized SVG route). The
// old image/* let the picker offer formats the server then refused, so you found out after choosing
const UPLOAD_ACCEPT =
  'image/jpeg,image/png,image/webp,image/gif,image/heic,image/heif,image/svg+xml,application/pdf'

export function MediaLibrary() {
  const [searchParams, setSearchParams] = useSearchParams()
  const path = searchParams.get('path') ?? ''
  const query = searchParams.get('q') ?? ''
  const { has } = usePermissions()
  const canManage = has('media.manage')

  const [uploadOpen, setUploadOpen] = useState(false)
  const [directoryOpen, setDirectoryOpen] = useState(false)
  const [selectedAsset, setSelectedAsset] = useState<MediaAsset | null>(null)
  const [selectedUntrackedFile, setSelectedUntrackedFile] = useState<UntrackedMediaFile | null>(null)
  const [errorMessage, setErrorMessage] = useState<string | null>(null)

  const directoryQuery = useQuery<MediaDirectoryQuery, MediaDirectoryQueryVariables>(
    MediaDirectoryDocument,
    { variables: { path } },
  )
  const searchQuery = useQuery<SearchMediaAssetsQuery, SearchMediaAssetsQueryVariables>(
    SearchMediaAssetsDocument,
    {
      variables: { query, path, limit: 100, offset: 0 },
      skip: query.trim() === '',
    },
  )
  const allDirsQuery = useQuery<AllMediaDirectoriesQuery>(AllMediaDirectoriesDocument)

  const refetchDirectory = [
    { query: MediaDirectoryDocument, variables: { path } },
    { query: AllMediaDirectoriesDocument },
  ]
  const [createDirectory, { loading: creatingDirectory }] = useMutation<
    CreateMediaDirectoryMutation,
    CreateMediaDirectoryMutationVariables
  >(CreateMediaDirectoryDocument, { refetchQueries: refetchDirectory })
  const [deleteDirectory] = useMutation<
    DeleteMediaDirectoryMutation,
    DeleteMediaDirectoryMutationVariables
  >(DeleteMediaDirectoryDocument)
  const [renameDirectory] = useMutation<
    RenameMediaDirectoryMutation,
    RenameMediaDirectoryMutationVariables
  >(RenameMediaDirectoryDocument)

  const [selectedDir, setSelectedDir] = useState<string | null>(null)

  // Clear selection whenever the URL path changes (tree / breadcrumb
  // navigation) - State is adjusted during render instead of in an effect
  const [prevPath, setPrevPath] = useState(path)
  if (prevPath !== path) {
    setPrevPath(path)
    setSelectedDir(null)
  }

  const selectedDirQuery = useQuery<MediaDirectoryQuery, MediaDirectoryQueryVariables>(
    MediaDirectoryDocument,
    { variables: { path: selectedDir ?? '' }, skip: selectedDir === null },
  )

  const directory = directoryQuery.data?.mediaDirectory
  // Files shown come from the selected directory card, falling back to current path
  const activeDir = selectedDir !== null
    ? selectedDirQuery.data?.mediaDirectory
    : directory
  const assets = useMemo(
    () =>
      query.trim() ? searchQuery.data?.searchMediaAssets.items ?? [] : activeDir?.assets ?? [],
    [query, searchQuery.data, activeDir],
  )
  const untrackedFiles = useMemo(
    () => (query.trim() ? [] : activeDir?.untrackedFiles ?? []),
    [query, activeDir],
  )

  const fileExtensions = useMemo(() => {
    const exts = [...new Set(
      assets
        .map((a) => { const i = a.filename.lastIndexOf('.'); return i !== -1 ? a.filename.slice(i + 1).toLowerCase() : null })
        .filter((e): e is string => e !== null && e.length > 0 && e.length <= 5),
    )].sort()
    return exts.join(', ')
  }, [assets])

  const untrackedExtensions = useMemo(() => {
    const exts = [...new Set(
      untrackedFiles
        .map((f) => { const i = f.filename.lastIndexOf('.'); return i !== -1 ? f.filename.slice(i + 1).toLowerCase() : null })
        .filter((e): e is string => e !== null && e.length > 0 && e.length <= 5),
    )].sort()
    return exts.join(', ')
  }, [untrackedFiles])

  const breadcrumbs = useMemo(() => buildBreadcrumbs(path), [path])
  const directoryTree = useMemo(() => {
    const dirs = allDirsQuery.data?.allMediaDirectories ?? []
    return buildDirectoryTree(dirs.map((d) => d.path))
  }, [allDirsQuery.data])

  function navigateTo(nextPath: string) {
    const params = new URLSearchParams(searchParams)
    if (nextPath) params.set('path', nextPath)
    else params.delete('path')
    setSearchParams(params)
  }

  function setQuery(nextQuery: string) {
    const params = new URLSearchParams(searchParams)
    if (nextQuery) {
      params.set('q', nextQuery)
      setSelectedDir(null)
    } else {
      params.delete('q')
    }
    setSearchParams(params)
  }

  async function onCreateDirectory(name: string) {
    setErrorMessage(null)
    try {
      await createDirectory({ variables: { path, name } })
      setDirectoryOpen(false)
    } catch (err) {
      setErrorMessage(err instanceof Error ? err.message : 'Create directory failed.')
    }
  }

  async function onRenameCurrentDirectory() {
    if (!path) return
    const currentName = path.split('/').at(-1) ?? path
    const nextName = window.prompt('Rename directory', currentName)
    if (!nextName || nextName === currentName) return
    setErrorMessage(null)
    try {
      const result = await renameDirectory({ variables: { path, newName: nextName } })
      navigateTo(result.data?.renameMediaDirectory.path ?? '')
    } catch (err) {
      setErrorMessage(err instanceof Error ? err.message : 'Rename directory failed.')
    }
  }

  async function onDeleteCurrentDirectory(recursive: boolean) {
    if (!path) return
    if (!window.confirm(recursive ? 'Delete this directory and all files inside it?' : 'Delete this empty directory?')) {
      return
    }
    setErrorMessage(null)
    try {
      await deleteDirectory({ variables: { path, recursive } })
      const parent = path.includes('/') ? path.split('/').slice(0, -1).join('/') : ''
      navigateTo(parent)
    } catch (err) {
      setErrorMessage(err instanceof Error ? err.message : 'Delete directory failed.')
    }
  }

  const totalDirCount = allDirsQuery.data?.allMediaDirectories.length ?? 0
  const totalCount = searchQuery.data?.searchMediaAssets.totalCount ?? assets.length
  const headerLabel = query
    ? `${totalCount} result${totalCount === 1 ? '' : 's'} for "${query}"`
    : null

  return (
    <div className="-m-4 flex h-[calc(100%+2rem)] overflow-hidden md:-m-6 md:h-[calc(100%+3rem)]">
      {/* ── Left: directory tree ─────────────────────────────── */}
      <aside className="flex w-80 shrink-0 flex-col border-r bg-card">
        <div className="flex flex-col gap-3 border-b px-4 py-3">
          <div className="flex items-center justify-between gap-2">
            <div className="flex items-baseline gap-2">
              <h2 className="text-sm font-semibold tracking-tight">Media</h2>
              <span className="text-xs text-muted-foreground">{totalDirCount} folder{totalDirCount === 1 ? '' : 's'}</span>
            </div>
            {canManage ? (
              <Button
                size="sm"
                variant="outline"
                className="h-7 gap-1 px-2 text-xs"
                onClick={() => setDirectoryOpen(true)}
              >
                <PlusIcon className="size-3.5" />
                New
              </Button>
            ) : null}
          </div>
          <div className="relative">
            <SearchIcon className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
            <Input
              className="h-8 pl-8 text-xs"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="Search filename, alt, title…"
            />
          </div>
        </div>

        <div role="tree" aria-label="Folders" className="scrollbar-accent flex-1 overflow-y-auto px-2 py-2">
          <RootRow
            isActive={path === '' && !query}
            tabbable={path === ''}
            onClick={() => navigateTo('')}
          />
          {directoryTree.length > 0 ? (
            <DirectoryTree
              nodes={directoryTree}
              activePath={path}
              onNavigate={navigateTo}
            />
          ) : null}
        </div>
      </aside>

      {/* ── Right: assets ─────────────────────────────────────── */}
      <div className="scrollbar-accent flex flex-1 flex-col overflow-y-auto">
        {/* Sticky action header */}
        <div className="sticky top-0 z-10 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/80">
          <div className="flex flex-wrap items-center justify-between gap-3 px-6 py-3">
            <div className="min-w-0 flex-1">
              <div className="flex flex-wrap items-center gap-1 text-sm">
                {breadcrumbs.map((crumb, index) => (
                  <span key={crumb.path || 'root'} className="inline-flex items-center gap-1">
                    {index > 0 ? <span className="text-muted-foreground">/</span> : null}
                    <button
                      type="button"
                      className={`rounded-sm px-1 transition-colors ${index === breadcrumbs.length - 1 ? 'font-semibold text-foreground' : 'text-muted-foreground hover:text-foreground'}`}
                      onClick={() => navigateTo(crumb.path)}
                    >
                      {crumb.name}
                    </button>
                  </span>
                ))}
              </div>
              <p className="mt-0.5 text-xs text-muted-foreground">
                {headerLabel
                  ?? `${assets.length} file${assets.length === 1 ? '' : 's'}${fileExtensions ? ` · ${fileExtensions}` : ''}`}
              </p>
            </div>
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
                      <DropdownMenuItem onSelect={onRenameCurrentDirectory}>
                        <PencilIcon className="size-4" />
                        Rename
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem variant="destructive" onSelect={() => onDeleteCurrentDirectory(false)}>
                        <Trash2Icon className="size-4" />
                        Delete empty
                      </DropdownMenuItem>
                      <DropdownMenuItem variant="destructive" onSelect={() => onDeleteCurrentDirectory(true)}>
                        <Trash2Icon className="size-4" />
                        Delete recursive
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                ) : null}
                <Button size="sm" onClick={() => setUploadOpen(true)}>
                  <UploadIcon className="size-4" />
                  Upload
                </Button>
              </div>
            ) : null}
          </div>
        </div>

        {/* Body: errors, images grid, untracked */}
        <div className="flex-1 px-6 pb-16 pt-5">
          {directoryQuery.error || searchQuery.error || errorMessage ? (
            <Alert variant="destructive" className="mb-5">
              <AlertTitle>Media library error</AlertTitle>
              <AlertDescription>
                {errorMessage ?? directoryQuery.error?.message ?? searchQuery.error?.message}
              </AlertDescription>
            </Alert>
          ) : null}

          {directoryQuery.loading && assets.length === 0 ? (
            <p className="text-sm text-muted-foreground">Loading…</p>
          ) : assets.length ? (
            <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-4 2xl:grid-cols-6">
              {assets.map((asset) => (
                <AssetCard key={asset.id} asset={asset} onSelect={() => setSelectedAsset(asset)} />
              ))}
            </div>
          ) : !query && untrackedFiles.length === 0 ? (
            <EmptyMediaState
              canManage={canManage}
              onUpload={() => setUploadOpen(true)}
            />
          ) : null}

          {untrackedFiles.length > 0 ? (
            <div className="mt-8 grid gap-3">
              <div className="flex items-center gap-2">
                <AlertTriangleIcon className="size-3.5 text-warning" />
                <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                  {untrackedFiles.length} untracked
                  {untrackedExtensions ? ` · ${untrackedExtensions}` : ''}
                </span>
                <div className="h-px flex-1 bg-border" />
              </div>
              <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-4 2xl:grid-cols-6">
                {untrackedFiles.map((file) => (
                  <UntrackedFileCard key={file.objectKey} file={file} onSelect={() => setSelectedUntrackedFile(file)} />
                ))}
              </div>
            </div>
          ) : null}
        </div>
      </div>

      <UntrackedFileSheet
        file={selectedUntrackedFile}
        onOpenChange={(open) => { if (!open) setSelectedUntrackedFile(null) }}
      />
      <CreateDirectoryDialog
        open={directoryOpen}
        loading={creatingDirectory}
        onOpenChange={setDirectoryOpen}
        onSubmit={onCreateDirectory}
      />
      <UploadMediaDialog
        open={uploadOpen}
        path={selectedDir ?? path}
        onOpenChange={setUploadOpen}
        onUploaded={() => {
          void directoryQuery.refetch()
          void searchQuery.refetch()
          if (selectedDir !== null) void selectedDirQuery.refetch()
        }}
        onError={setErrorMessage}
      />
      <AssetDetailsSheet
        asset={selectedAsset}
        canManage={canManage}
        currentPath={path}
        onOpenChange={(open) => {
          if (!open) setSelectedAsset(null)
        }}
        onChanged={(asset) => {
          setSelectedAsset(asset)
          void directoryQuery.refetch()
          void searchQuery.refetch()
          if (selectedDir !== null) void selectedDirQuery.refetch()
        }}
        onDeleted={() => {
          setSelectedAsset(null)
          void directoryQuery.refetch()
          void searchQuery.refetch()
          if (selectedDir !== null) void selectedDirQuery.refetch()
        }}
        onError={setErrorMessage}
      />
    </div>
  )
}

// The tree's one tab stop is the open folder's row - Library's when the root is open, a search included
function RootRow({ isActive, tabbable, onClick }: { isActive: boolean; tabbable: boolean; onClick: () => void }) {
  return (
    <div
      role="treeitem"
      tabIndex={tabbable ? 0 : -1}
      aria-level={1}
      aria-selected={isActive}
      onClick={onClick}
      onKeyDown={(e) => onTreeItemKeyDown(e, { expanded: true, hasChildren: false, onToggle: () => {}, onOpen: onClick })}
      className={`mb-1 flex w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring ${
        isActive ? 'bg-primary/10 font-medium text-foreground' : 'text-foreground/90 hover:bg-muted/60'
      }`}
    >
      <FolderIcon className={`size-4 shrink-0 ${isActive ? 'text-primary' : 'text-muted-foreground'}`} />
      <span className="truncate">Library</span>
    </div>
  )
}

function EmptyMediaState({
  canManage,
  onUpload,
}: {
  canManage: boolean
  onUpload: () => void
}) {
  return (
    <div className="flex min-h-[40vh] flex-col items-center justify-center gap-3 rounded-lg border border-dashed p-12 text-center">
      <div className="grid size-12 place-items-center rounded-full bg-muted">
        <FolderOpenIcon className="size-6 text-muted-foreground" />
      </div>
      <p className="text-sm font-medium">No media in this folder</p>
      <p className="max-w-xs text-xs text-muted-foreground">
        Upload images and other assets, or pick a different folder from the tree on the left.
      </p>
      {canManage ? (
        <Button size="sm" onClick={onUpload}>
          <UploadIcon className="size-4" /> Upload media
        </Button>
      ) : null}
    </div>
  )
}

function AssetCard({ asset, onSelect }: { asset: MediaAsset; onSelect: () => void }) {
  return (
    <button
      type="button"
      onClick={onSelect}
      className="group relative aspect-square overflow-hidden rounded-lg border bg-muted text-left transition-shadow hover:shadow-md focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
    >
      {asset.mimeGroup === 'IMAGE' ? (
        <img
          src={asset.url}
          alt={asset.altText || asset.filename}
          loading="lazy"
          className="size-full object-cover transition-transform duration-200 group-hover:scale-[1.02]"
        />
      ) : (
        <div className="grid size-full place-items-center text-muted-foreground">
          <FileTextIcon className="size-10" />
        </div>
      )}
      {asset.status !== 'READY' ? (
        <Badge variant="outline" className="absolute left-2 top-2 bg-background/85 backdrop-blur">
          {asset.status.toLowerCase()}
        </Badge>
      ) : null}
      <div className="pointer-events-none absolute inset-x-0 bottom-0 translate-y-full bg-gradient-to-t from-black/80 via-black/40 to-transparent p-2.5 text-white opacity-0 transition-all duration-200 group-hover:translate-y-0 group-hover:opacity-100">
        <p className="truncate text-xs font-medium">{asset.title || asset.filename}</p>
        <p className="truncate text-[10px] text-white/70">
          {formatBytes(asset.sizeBytes)} · {asset.mimeType.split('/')[1]?.toUpperCase() ?? asset.mimeType}
        </p>
      </div>
    </button>
  )
}

function UntrackedFileCard({ file, onSelect }: { file: UntrackedMediaFile; onSelect: () => void }) {
  const isImage = file.mimeType.startsWith('image/')
  return (
    <button
      type="button"
      onClick={onSelect}
      className="group relative aspect-square overflow-hidden rounded-lg border border-warning/40 bg-muted text-left transition-shadow hover:shadow-md focus:outline-none focus-visible:ring-2 focus-visible:ring-warning"
    >
      {isImage ? (
        <img
          src={file.url}
          alt={file.filename}
          loading="lazy"
          className="size-full object-cover opacity-80 transition-transform duration-200 group-hover:scale-[1.02]"
        />
      ) : (
        <div className="grid size-full place-items-center text-muted-foreground">
          <FileTextIcon className="size-10" />
        </div>
      )}
      <Badge variant="outline" className="absolute left-2 top-2 bg-background/85 text-warning backdrop-blur">
        untracked
      </Badge>
      <div className="pointer-events-none absolute inset-x-0 bottom-0 translate-y-full bg-gradient-to-t from-black/80 via-black/40 to-transparent p-2.5 text-white opacity-0 transition-all duration-200 group-hover:translate-y-0 group-hover:opacity-100">
        <p className="truncate text-xs font-medium">{file.filename}</p>
        <p className="truncate text-[10px] text-white/70">
          {formatBytes(file.sizeBytes)} · {file.mimeType.split('/')[1]?.toUpperCase() ?? file.mimeType}
        </p>
      </div>
    </button>
  )
}

function UntrackedFileSheet({
  file,
  onOpenChange,
}: {
  file: UntrackedMediaFile | null
  onOpenChange: (open: boolean) => void
}) {
  const isImage = file?.mimeType.startsWith('image/') ?? false
  return (
    <Sheet open={!!file} onOpenChange={onOpenChange}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-xl">
        {file ? (
          <>
            <SheetHeader>
              <SheetTitle>{file.filename}</SheetTitle>
            </SheetHeader>
            <div className="grid gap-4 px-4">
              <div className="overflow-hidden rounded-lg border bg-muted">
                {isImage ? (
                  <img src={file.url} alt={file.filename} className="max-h-80 w-full object-contain" />
                ) : (
                  <div className="grid aspect-video place-items-center text-muted-foreground">
                    <FileTextIcon className="size-12" />
                  </div>
                )}
              </div>
              <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                <Badge variant="outline" className="text-warning">untracked</Badge>
                <Badge variant="outline">{file.mimeType}</Badge>
                <span>{formatBytes(file.sizeBytes)}</span>
                <Button variant="ghost" size="xs" asChild>
                  <a href={file.url} target="_blank" rel="noreferrer">
                    <ExternalLinkIcon className="size-3" />
                    Open
                  </a>
                </Button>
              </div>
              <div className="grid gap-1 text-sm">
                <p className="text-xs font-medium text-muted-foreground">Storage key</p>
                <p className="break-all font-mono text-xs">{file.objectKey}</p>
              </div>
              <div className="grid gap-1 text-sm">
                <p className="text-xs font-medium text-muted-foreground">Last modified</p>
                <p className="text-xs">{new Date(file.lastModified).toLocaleString()}</p>
              </div>
              <Alert>
                <AlertTriangleIcon className="size-4" />
                <AlertTitle>No library record</AlertTitle>
                <AlertDescription>
                  This file exists in storage but is not tracked in the media library database.
                  It may be an orphaned upload or was added directly to storage.
                </AlertDescription>
              </Alert>
            </div>
          </>
        ) : null}
      </SheetContent>
    </Sheet>
  )
}

function CreateDirectoryDialog({
  open,
  loading,
  onOpenChange,
  onSubmit,
}: {
  open: boolean
  loading: boolean
  onOpenChange: (open: boolean) => void
  onSubmit: (name: string) => Promise<void>
}) {
  const [name, setName] = useState('')

  // Reset the field each time the dialog opens - State is adjusted during
  // render instead of in an effect
  const [prevOpen, setPrevOpen] = useState(open)
  if (prevOpen !== open) {
    setPrevOpen(open)
    if (open) setName('')
  }

  async function handleSubmit(event: FormEvent) {
    event.preventDefault()
    if (!name.trim()) return
    await onSubmit(name.trim())
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form className="grid gap-4" onSubmit={handleSubmit}>
          <DialogHeader>
            <DialogTitle>Create directory</DialogTitle>
          </DialogHeader>
          <div className="grid gap-2">
            <Label htmlFor="media-directory-name">Name</Label>
            <Input id="media-directory-name" value={name} onChange={(event) => setName(event.target.value)} autoFocus />
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={loading || !name.trim()}>
              Create
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function UploadMediaDialog({
  open,
  path,
  onOpenChange,
  onUploaded,
  onError,
}: {
  open: boolean
  path: string
  onOpenChange: (open: boolean) => void
  onUploaded: () => void
  onError: (message: string | null) => void
}) {
  const fileInputRef = useRef<HTMLInputElement | null>(null)
  const [file, setFile] = useState<File | null>(null)
  const [title, setTitle] = useState('')
  const [altText, setAltText] = useState('')
  const [caption, setCaption] = useState('')
  const [notes, setNotes] = useState('')
  const [uploading, setUploading] = useState(false)

  const [requestUpload] = useMutation<RequestMediaAssetUploadMutation, RequestMediaAssetUploadMutationVariables>(
    RequestMediaAssetUploadDocument,
  )
  const [confirmUpload] = useMutation<ConfirmMediaAssetUploadMutation, ConfirmMediaAssetUploadMutationVariables>(
    ConfirmMediaAssetUploadDocument,
  )

  // Reset the form each time the dialog opens - State is adjusted during
  // render instead of in an effect
  const [prevOpen, setPrevOpen] = useState(open)
  if (prevOpen !== open) {
    setPrevOpen(open)
    if (open) {
      setFile(null)
      setTitle('')
      setAltText('')
      setCaption('')
      setNotes('')
    }
  }

  // The native file input holds browser-owned state React state can't
  // reach, so it is cleared in an effect (external-system sync)
  useEffect(() => {
    if (!open) return
    if (fileInputRef.current) fileInputRef.current.value = ''
  }, [open])

  function onFileChange(event: ChangeEvent<HTMLInputElement>) {
    const nextFile = event.target.files?.[0] ?? null
    setFile(nextFile)
    if (nextFile && !title) setTitle(nextFile.name)
  }

  async function handleSubmit(event: FormEvent) {
    event.preventDefault()
    if (!file) return
    setUploading(true)
    onError(null)
    try {
      // SVG is sanitized server-side, so its bytes go to the app rather than straight to S3 on a
      // presigned URL - same reason as MediaImageInput
      if (file.type === 'image/svg+xml') {
        const body = new FormData()
        body.append('file', file)
        body.append('path', path)
        if (title) body.append('title', title)
        if (altText) body.append('altText', altText)
        const token = getAuthStoredState()?.accessToken
        const res = await fetch(ADMIN_MEDIA_SVG_ENDPOINT, {
          method: 'POST',
          headers: token ? { Authorization: `Bearer ${token}` } : undefined,
          body,
        })
        const payload = (await res.json().catch(() => null)) as { id?: string; error?: string } | null
        if (!res.ok || !payload?.id) throw new Error(payload?.error ?? `Upload failed with HTTP ${res.status}`)
        onUploaded()
        onOpenChange(false)
        return
      }
      const requested = await requestUpload({
        variables: {
          input: {
            path,
            filename: file.name,
            mimeType: file.type,
            sizeBytes: file.size,
            title,
            altText,
            caption,
            notes,
          },
        },
      })
      const payload = requested.data?.requestMediaAssetUpload
      if (!payload) throw new Error('Upload request did not return a payload.')

      const uploaded = await fetch(payload.uploadUrl, {
        method: 'PUT',
        headers: { 'Content-Type': file.type },
        body: file,
      })
      if (!uploaded.ok) throw new Error(`Upload failed with HTTP ${uploaded.status}`)

      const dimensions: { width?: number; height?: number } =
        file.type.startsWith('image/') ? await readImageDimensions(file) : {}
      await confirmUpload({
        variables: {
          assetId: payload.asset.id,
          width: dimensions.width ?? null,
          height: dimensions.height ?? null,
        },
      })
      onUploaded()
      onOpenChange(false)
    } catch (err) {
      onError(err instanceof Error ? err.message : 'Upload failed.')
    } finally {
      setUploading(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <form className="grid gap-4" onSubmit={handleSubmit}>
          <DialogHeader>
            <DialogTitle>Upload media</DialogTitle>
          </DialogHeader>
          <div className="grid gap-2">
            <Label htmlFor="media-file">File</Label>
            <Input id="media-file" ref={fileInputRef} type="file" accept={UPLOAD_ACCEPT} onChange={onFileChange} />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="media-title">Title</Label>
            <Input id="media-title" value={title} onChange={(event) => setTitle(event.target.value)} />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="media-alt">Alt text</Label>
            <Input id="media-alt" value={altText} onChange={(event) => setAltText(event.target.value)} />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="media-caption">Caption</Label>
            <Textarea id="media-caption" value={caption} onChange={(event) => setCaption(event.target.value)} rows={2} />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="media-notes">Notes</Label>
            <Textarea id="media-notes" value={notes} onChange={(event) => setNotes(event.target.value)} rows={2} />
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={!file || uploading}>
              {uploading ? 'Uploading...' : 'Upload'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

export function AssetDetailsSheet({
  asset,
  canManage,
  currentPath,
  onOpenChange,
  onChanged,
  onDeleted,
  onError,
}: {
  asset: MediaAsset | null
  canManage: boolean
  currentPath: string
  onOpenChange: (open: boolean) => void
  onChanged: (asset: MediaAsset) => void
  onDeleted: () => void
  onError: (message: string | null) => void
}) {
  const [title, setTitle] = useState(asset?.title ?? '')
  const [altText, setAltText] = useState(asset?.altText ?? '')
  const [caption, setCaption] = useState(asset?.caption ?? '')
  const [notes, setNotes] = useState(asset?.notes ?? '')
  const [filename, setFilename] = useState(asset?.filename ?? '')
  const [targetPath, setTargetPath] = useState(asset?.directoryPath ?? '')
  const [saving, setSaving] = useState(false)

  // Re-hydrate the form whenever a different asset object is selected (state adjusted during render instead of in an
  // effect) - The last values are kept while the sheet closes (asset -> null), as before
  const [prevAsset, setPrevAsset] = useState(asset)
  if (prevAsset !== asset) {
    setPrevAsset(asset)
    if (asset) {
      setTitle(asset.title)
      setAltText(asset.altText)
      setCaption(asset.caption)
      setNotes(asset.notes)
      setFilename(asset.filename)
      setTargetPath(asset.directoryPath)
    }
  }

  // Closing the sheet used to drop typed-in metadata without a word - it now asks, and leaving the page is guarded too
  const dirty =
    !!asset &&
    (title !== asset.title ||
      altText !== asset.altText ||
      caption !== asset.caption ||
      notes !== asset.notes ||
      filename !== asset.filename ||
      targetPath !== asset.directoryPath)
  useUnsavedChangesGuard(dirty, 'Media asset')
  const [confirmingDiscard, setConfirmingDiscard] = useState(false)
  function requestOpenChange(open: boolean) {
    if (!open && dirty) setConfirmingDiscard(true)
    else onOpenChange(open)
  }

  const [updateAsset] = useMutation<UpdateMediaAssetMutation, UpdateMediaAssetMutationVariables>(
    UpdateMediaAssetDocument,
  )
  const [setFocal] = useMutation<SetMediaFocalPointMutation, SetMediaFocalPointMutationVariables>(
    SetMediaFocalPointDocument,
  )

  // Click-to-set crop focal point - The wrapper shrink-wraps the rendered
  // image, so the element box IS the bitmap box and fractions map 1:1
  async function applyFocal(x: number | null, y: number | null) {
    if (!asset) return
    onError(null)
    try {
      const res = await setFocal({ variables: { assetId: asset.id, x, y } })
      if (res.data?.setMediaFocalPoint) onChanged(res.data.setMediaFocalPoint)
    } catch (err) {
      onError(err instanceof Error ? err.message : 'Setting focal point failed.')
    }
  }

  function onPickFocal(e: ReactMouseEvent<HTMLImageElement>) {
    const rect = e.currentTarget.getBoundingClientRect()
    const x = Math.min(1, Math.max(0, (e.clientX - rect.left) / rect.width))
    const y = Math.min(1, Math.max(0, (e.clientY - rect.top) / rect.height))
    void applyFocal(Number(x.toFixed(4)), Number(y.toFixed(4)))
  }
  const [moveAsset] = useMutation<MoveMediaAssetMutation, MoveMediaAssetMutationVariables>(MoveMediaAssetDocument)
  const [deleteAsset] = useMutation<DeleteMediaAssetMutation, DeleteMediaAssetMutationVariables>(
    DeleteMediaAssetDocument,
  )
  const { data: aiStatusData } = useQuery<MediaAiStatusQuery>(MediaAiStatusDocument)
  const aiEnabled = aiStatusData?.aiStatus.enabled ?? false
  const [genAlt, { loading: genningAlt }] = useMutation<
    GenerateMediaAltTextMutation,
    GenerateMediaAltTextMutationVariables
  >(GenerateMediaAltTextDocument)

  async function onGenerateAlt() {
    if (!asset) return
    onError(null)
    try {
      const res = await genAlt({ variables: { assetId: asset.id } })
      if (res.data?.generateMediaAltText) setAltText(res.data.generateMediaAltText)
    } catch (e) {
      onError(e instanceof Error ? e.message : 'Alt-text generation failed.')
    }
  }

  async function onSave() {
    if (!asset) return
    setSaving(true)
    onError(null)
    try {
      const updated = await updateAsset({
        variables: { input: { id: asset.id, title, altText, caption, notes } },
      })
      let next = updated.data?.updateMediaAsset
      if (targetPath !== asset.directoryPath || filename !== asset.filename) {
        const moved = await moveAsset({
          variables: {
            id: asset.id,
            targetPath,
            filename,
          },
        })
        next = moved.data?.moveMediaAsset ?? next
      }
      if (next) onChanged(next)
    } catch (err) {
      onError(err instanceof Error ? err.message : 'Save media asset failed.')
    } finally {
      setSaving(false)
    }
  }

  async function onDelete() {
    if (!asset) return
    if (!window.confirm('Delete this media asset?')) return
    onError(null)
    try {
      await deleteAsset({ variables: { id: asset.id, deleteFromStorage: true } })
      onDeleted()
    } catch (err) {
      onError(err instanceof Error ? err.message : 'Delete media asset failed.')
    }
  }

  return (
    <>
      <ConfirmDialog
        open={confirmingDiscard}
        onOpenChange={setConfirmingDiscard}
        title="Discard changes to this asset?"
        description="The title, alt text, caption, notes or location you edited haven't been saved."
        confirmLabel="Discard"
        destructive
        onConfirm={() => onOpenChange(false)}
      />
      <Sheet open={!!asset} onOpenChange={requestOpenChange}>
        <SheetContent className="w-full overflow-y-auto sm:max-w-xl">
          {asset ? (
            <>
              <SheetHeader>
                <SheetTitle>{asset.title || asset.filename}</SheetTitle>
              </SheetHeader>
              <div className="grid gap-4 px-4">
                <div className="overflow-hidden rounded-lg border bg-muted">
                  {asset.mimeGroup === 'IMAGE' ? (
                    <div className="flex justify-center">
                      <div className="relative">
                        <img
                          src={asset.url}
                          alt={asset.altText || asset.filename}
                          className={cn('max-h-80 max-w-full', canManage && 'cursor-crosshair')}
                          onClick={canManage ? onPickFocal : undefined}
                        />
                        {asset.focalX != null && asset.focalY != null && (
                          <span
                            className="pointer-events-none absolute size-5 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-white shadow-md ring-1 ring-black/40"
                            style={{ left: `${asset.focalX * 100}%`, top: `${asset.focalY * 100}%` }}
                          />
                        )}
                      </div>
                    </div>
                  ) : (
                    <div className="grid aspect-video place-items-center text-muted-foreground">
                      <FileTextIcon className="size-12" />
                    </div>
                  )}
                </div>
                {asset.mimeGroup === 'IMAGE' && canManage && (
                  <p className="text-xs text-muted-foreground">
                    Click the image to set the crop focal point - server-side cover crops stay centered on it.{' '}
                    {asset.focalX != null && asset.focalY != null ? (
                      <>
                        <span className="tabular-nums">
                          ({Math.round(asset.focalX * 100)}%, {Math.round(asset.focalY * 100)}%)
                        </span>{' '}
                        <button type="button" className="underline underline-offset-2" onClick={() => void applyFocal(null, null)}>
                          Clear
                        </button>
                      </>
                    ) : (
                      <span>None set - crops use the center.</span>
                    )}
                  </p>
                )}
                <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                  <Badge variant="outline">{asset.mimeType}</Badge>
                  <span>{formatBytes(asset.sizeBytes)}</span>
                  {asset.width && asset.height ? <span>{asset.width}x{asset.height}</span> : null}
                  <Button variant="ghost" size="xs" asChild>
                    <a href={asset.url} target="_blank" rel="noreferrer">
                      <ExternalLinkIcon className="size-3" />
                      Open
                    </a>
                  </Button>
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="asset-title">Title</Label>
                  <Input id="asset-title" value={title} onChange={(event) => setTitle(event.target.value)} disabled={!canManage} />
                </div>
                <div className="grid gap-2">
                  <div className="flex items-center justify-between gap-2">
                    <Label htmlFor="asset-alt">Alt text</Label>
                    {aiEnabled && canManage && asset?.mimeGroup === 'IMAGE' && (
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        className="h-7 px-2 text-xs"
                        disabled={genningAlt}
                        onClick={onGenerateAlt}
                        title="Generate alt text from the image with AI"
                      >
                        <SparklesIcon className="size-3.5" />
                        {genningAlt ? 'Generating…' : 'Generate'}
                      </Button>
                    )}
                  </div>
                  <Input id="asset-alt" value={altText} onChange={(event) => setAltText(event.target.value)} disabled={!canManage} />
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="asset-caption">Caption</Label>
                  <Textarea id="asset-caption" value={caption} onChange={(event) => setCaption(event.target.value)} rows={3} disabled={!canManage} />
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="asset-notes">Notes</Label>
                  <Textarea id="asset-notes" value={notes} onChange={(event) => setNotes(event.target.value)} rows={3} disabled={!canManage} />
                </div>
                <div className="grid gap-3 sm:grid-cols-2">
                  <div className="grid gap-2">
                    <Label htmlFor="asset-path">Directory path</Label>
                    <Input id="asset-path" value={targetPath} onChange={(event) => setTargetPath(event.target.value)} disabled={!canManage} placeholder={currentPath} />
                  </div>
                  <div className="grid gap-2">
                    <Label htmlFor="asset-filename">Filename</Label>
                    <Input id="asset-filename" value={filename} onChange={(event) => setFilename(event.target.value)} disabled={!canManage} />
                  </div>
                </div>
              </div>
              {canManage ? (
                <SheetFooter>
                  <Button onClick={onSave} disabled={saving}>
                    {saving ? 'Saving...' : 'Save'}
                  </Button>
                  <Button variant="destructive" onClick={onDelete}>
                    <Trash2Icon className="size-4" />
                    Delete
                  </Button>
                </SheetFooter>
              ) : null}
            </>
          ) : null}
        </SheetContent>
      </Sheet>
    </>
  )
}

type DirTreeNode = {
  name: string
  path: string
  children: DirTreeNode[]
}

function buildDirectoryTree(paths: string[]): DirTreeNode[] {
  const root: DirTreeNode[] = []
  for (const dirPath of [...paths].sort()) {
    const parts = dirPath.split('/').filter(Boolean)
    let nodes = root
    for (let i = 0; i < parts.length; i++) {
      const fullPath = parts.slice(0, i + 1).join('/')
      let node = nodes.find((n) => n.name === parts[i])
      if (!node) {
        node = { name: parts[i], path: fullPath, children: [] }
        nodes.push(node)
      }
      nodes = node.children
    }
  }
  return root
}

function DirectoryTree({
  nodes,
  activePath,
  onNavigate,
  depth = 0,
}: {
  nodes: DirTreeNode[]
  activePath: string
  onNavigate: (path: string) => void
  depth?: number
}) {
  return (
    <ul role={depth > 0 ? 'group' : 'none'} className="grid gap-0.5">
      {nodes.map((node) => (
        <DirectoryTreeNode key={node.path} node={node} activePath={activePath} onNavigate={onNavigate} depth={depth} />
      ))}
    </ul>
  )
}

function DirectoryTreeNode({
  node,
  activePath,
  onNavigate,
  depth,
}: {
  node: DirTreeNode
  activePath: string
  onNavigate: (path: string) => void
  depth: number
}) {
  const isActive = node.path === activePath
  const isAncestor = activePath.startsWith(node.path + '/')
  const [open, setOpen] = useState(isAncestor || isActive)
  const hasChildren = node.children.length > 0

  // Auto-expand when the node becomes the active path or an ancestor of it - State is adjusted during render instead of in an effect; both
  // flags are tracked separately so an active->ancestor transition still re-expands a manually collapsed node
  const [prevActivity, setPrevActivity] = useState({ isAncestor, isActive })
  if (prevActivity.isAncestor !== isAncestor || prevActivity.isActive !== isActive) {
    setPrevActivity({ isAncestor, isActive })
    if (isAncestor || isActive) setOpen(true)
  }

  return (
    <li role="none">
      <div
        role="treeitem"
        tabIndex={isActive ? 0 : -1}
        aria-level={depth + 2}
        aria-selected={isActive}
        aria-expanded={hasChildren ? open : undefined}
        onKeyDown={(e) => onTreeItemKeyDown(e, { expanded: open, hasChildren, onToggle: () => setOpen((v) => !v), onOpen: () => onNavigate(node.path) })}
        className={`group relative flex cursor-pointer items-center gap-1 rounded-md py-1.5 pr-2 text-sm transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring ${
          isActive
            ? 'bg-primary/10 font-medium text-foreground'
            : isAncestor
              ? 'text-foreground hover:bg-muted/60'
              : 'text-foreground/90 hover:bg-muted/60'
        }`}
        style={{ paddingLeft: `${depth * 1.125 + 0.5}rem` }}
        onClick={() => onNavigate(node.path)}
      >
        {/* indent guides */}
        {depth > 0
          ? Array.from({ length: depth }).map((_, i) => (
              <span
                key={i}
                aria-hidden
                className="pointer-events-none absolute top-0 bottom-0 w-px bg-border/60"
                style={{ left: `${i * 1.125 + 0.875}rem` }}
              />
            ))
          : null}
        {/* The row's arrow keys expand and collapse, so this stays a mouse target out of the tab order */}
        <button
          type="button"
          tabIndex={-1}
          className="grid size-4 shrink-0 place-items-center text-muted-foreground hover:text-foreground"
          onClick={(e) => { e.stopPropagation(); setOpen((v) => !v) }}
          aria-label={open ? `Collapse ${node.name}` : `Expand ${node.name}`}
          disabled={!hasChildren}
        >
          {hasChildren ? (
            open ? <ChevronDownIcon className="size-3.5" /> : <ChevronRightIcon className="size-3.5" />
          ) : null}
        </button>
        {open && hasChildren ? (
          <FolderOpenIcon className={`size-4 shrink-0 ${isActive ? 'text-primary' : 'text-muted-foreground'}`} />
        ) : (
          <FolderIcon className={`size-4 shrink-0 ${isActive ? 'text-primary' : 'text-muted-foreground'}`} />
        )}
        <span className="flex-1 truncate">{node.name}</span>
      </div>
      {hasChildren && open ? (
        <DirectoryTree nodes={node.children} activePath={activePath} onNavigate={onNavigate} depth={depth + 1} />
      ) : null}
    </li>
  )
}

function buildBreadcrumbs(path: string): { name: string; path: string }[] {
  const crumbs = [{ name: 'Library', path: '' }]
  if (!path) return crumbs
  const parts = path.split('/').filter(Boolean)
  for (let i = 0; i < parts.length; i += 1) {
    crumbs.push({ name: parts[i], path: parts.slice(0, i + 1).join('/') })
  }
  return crumbs
}

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  let size = bytes
  let unit = 0
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024
    unit += 1
  }
  return `${size.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`
}

