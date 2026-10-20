import { useState } from 'react'
import { CheckIcon, CopyIcon, LayoutGridIcon, PencilIcon, PlusIcon, TrashIcon, ColumnsIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'
import { useWorkspaceTabs } from '@/lib/workspace-tabs-context'
import { useConfirmDialog } from '@/shared/dialogs/useConfirmDialog'

// Workspaces are whole saved arrangements - their own tabs, their own split layout. The switcher is
// the one place they are made, named, copied and thrown away, and it also holds the layout actions
// that only make sense once a workspace is split

type Prompt = { mode: 'create' | 'duplicate' | 'rename'; value: string }

const COPY = {
  create: { title: 'New workspace', blurb: 'A fresh workspace starts on your landing page with a single tab.', action: 'Create' },
  duplicate: { title: 'Duplicate workspace', blurb: 'Copies this workspace’s tabs and split layout, and leaves the original as it is.', action: 'Duplicate' },
  rename: { title: 'Rename workspace', blurb: 'Only the name changes - tabs and layout stay put.', action: 'Rename' },
} as const

function NamePrompt({ prompt, onClose, onSubmit }: { prompt: Prompt | null; onClose: () => void; onSubmit: (name: string) => void }) {
  const [name, setName] = useState('')
  const copy = prompt ? COPY[prompt.mode] : COPY.create

  return (
    <Dialog
      open={prompt !== null}
      onOpenChange={(open) => {
        if (!open) onClose()
      }}
    >
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>{copy.title}</DialogTitle>
          <DialogDescription>{copy.blurb}</DialogDescription>
        </DialogHeader>
        <form
          onSubmit={(e) => {
            e.preventDefault()
            onSubmit(name)
            onClose()
          }}
        >
          <Input
            autoFocus
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={prompt?.value}
            aria-label="Workspace name"
          />
          <DialogFooter className="mt-4">
            <Button type="button" variant="ghost" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit">{copy.action}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

export function WorkspaceSwitcher() {
  const [confirm, confirmDialog] = useConfirmDialog()
  const {
    workspaces,
    workspaceId,
    workspaceName,
    switchWorkspace,
    createWorkspace,
    duplicateWorkspace,
    renameWorkspace,
    deleteWorkspace,
    nextWorkspaceName,
    mergePanes,
    panes,
  } = useWorkspaceTabs()
  const [prompt, setPrompt] = useState<Prompt | null>(null)

  const submit = (name: string) => {
    if (!prompt) return
    if (prompt.mode === 'create') createWorkspace(name || prompt.value)
    else if (prompt.mode === 'duplicate') duplicateWorkspace(name || prompt.value)
    else renameWorkspace(workspaceId, name || prompt.value)
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            className="h-8 max-w-40 shrink-0 gap-1.5 px-2 text-muted-foreground"
            title={`Workspace: ${workspaceName}`}
          >
            <LayoutGridIcon className="size-4 shrink-0" />
            {/* the name is the useful half, but a phone header has no room for it */}
            <span className="hidden truncate sm:inline">{workspaceName}</span>
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="w-60">
          <DropdownMenuLabel>Workspaces</DropdownMenuLabel>
          {workspaces.map((w) => (
            <DropdownMenuItem key={w.id} onSelect={() => switchWorkspace(w.id)}>
              <CheckIcon className={cn('size-4', w.id !== workspaceId && 'invisible')} />
              <span className="min-w-0 flex-1 truncate">{w.name}</span>
            </DropdownMenuItem>
          ))}
          <DropdownMenuSeparator />
          <DropdownMenuItem onSelect={() => setPrompt({ mode: 'create', value: nextWorkspaceName() })}>
            <PlusIcon className="size-4" />
            New workspace
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={() => setPrompt({ mode: 'duplicate', value: `${workspaceName} copy` })}>
            <CopyIcon className="size-4" />
            Duplicate
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={() => setPrompt({ mode: 'rename', value: workspaceName })}>
            <PencilIcon className="size-4" />
            Rename
          </DropdownMenuItem>
          {panes.length > 1 && (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuItem onSelect={mergePanes}>
                <ColumnsIcon className="size-4" />
                Merge panes
              </DropdownMenuItem>
            </>
          )}
          {workspaces.length > 1 && (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                variant="destructive"
                onSelect={() =>
                  confirm({
                    title: `Delete "${workspaceName}" and its tabs?`,
                    confirmLabel: 'Delete workspace',
                    destructive: true,
                    onConfirm: () => deleteWorkspace(workspaceId),
                  })
                }
              >
                <TrashIcon className="size-4" />
                Delete workspace
              </DropdownMenuItem>
            </>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
      {/* keyed so the input starts empty on every open, rather than holding the last name typed */}
      <NamePrompt key={prompt?.mode ?? 'closed'} prompt={prompt} onClose={() => setPrompt(null)} onSubmit={submit} />
      {confirmDialog}
    </>
  )
}
