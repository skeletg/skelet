import { useMemo, useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { useUnsavedFields } from '@/lib/unsaved-changes'
import { duringCommit } from '@/lib/nav-guard'
import { useMutation, useQuery } from '@apollo/client/react'
import { DndContext } from '@dnd-kit/core'
import { arrayMove } from '@dnd-kit/sortable'
import {
  HistoryIcon,
  Redo2Icon,
  SparklesIcon,
  Undo2Icon,
} from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { DetailPageHeader, DetailPageLayout } from '@/components/layout/detail-page'
import { LoadingState } from '@/components/ui/loading-state'
import { useConfigScope } from '@/lib/config-scope'
import { CampaignMergeTagsDocument } from '@/features/campaigns/Campaigns.gql'
import { FlowMergeTagsDocument } from '@/features/flows/Flows.gql'
import { BlockPalette } from '@/features/email/BlockPalette'
import { PreviewDialog } from './PreviewDialog'
import { EmailSizeMeter } from './EmailSizeMeter'
import { EmailCanvas, type CanvasItem } from '@/features/email/EmailCanvas'
import { BlockInspector, type AiRewriteFn } from '@/features/email/BlockInspector'
import { BlockStyleControls, GlobalStyleControls } from '@/features/email/StyleControls'
import { RowInspector } from '@/features/email/RowInspector'
import { FLOATING_PANEL, PANEL_BACKDROP, PANEL_TITLE } from '@/features/email/panel'
import { MergeTagMenu } from '@/features/email/MergeTagMenu'
import { ShortcutsHelp } from '@/features/email/ShortcutsHelp'
import { insertAtCaret } from '@/features/email/caret'
import { useCanvasShortcuts } from '@/features/email/shortcuts'
import { useCanvasDnd } from '@/features/email/canvasDnd'
import {
  childSel,
  duplicateBlock,
  findBlock,
  moveChild,
  parseSel,
  removeBlock,
  replaceBlock,
} from '@/features/email/treeops'
import {
  componentMap,
  newBlock,
  newBlockRef,
  shortPreview,
  type BlockComponent,
  type EmailBlock,
  type EmailStyles,
} from '@/features/email/blocks'
import { EmailBlocksDocument, SaveEmailBlockDocument } from '@/features/email/Email.gql'
import { toComponent } from '@/features/blocks/model'
import {
  DeleteEmailTranslationDocument,
  EmailGroupsDocument,
  EmailPreflightDocument,
  EmailTranslationsDocument,
  EmailRevisionsDocument,
  EmailsAiStatusDocument,
  LibraryEmailDocument,
  RestoreEmailRevisionDocument,
  RewriteEmailTextDocument,
  SaveEmailTranslationDocument,
  SaveLibraryEmailDocument,
  SuggestEmailSubjectsDocument,
} from './Emails.gql'
import { LoadError } from '@/shared/errors/LoadError'
import { useConfirmDialog } from '@/shared/dialogs/useConfirmDialog'
import { NotFound } from '@/shared/errors/NotFound'

// A hot reload restarts the counter, not the open editor - the prefix keeps new ids off old ones, which select together
const LOAD = Math.random().toString(36).slice(2, 7)
let uid = 0
const nextId = () => `le_${LOAD}_${++uid}`

// The sentinel Select value for "no group" - Radix Select forbids ""
const NO_GROUP = '__none__'

// Merge tags offered in the inspector's personalization inserter Merge tags come from the server (campaignMergeTags + flowMergeTags) - a
// library email can be sent from either a campaign or a flow, so only the union is offerable and a hardcoded list drifts
function useMergeTags(): string[] {
  const campaign = useQuery(CampaignMergeTagsDocument)
  const flow = useQuery(FlowMergeTagsDocument)
  return useMemo(() => {
    const all = [
      ...(campaign.data?.campaignMergeTags ?? []),
      ...(flow.data?.flowMergeTags ?? []),
    ]
    return [...new Set(all)]
  }, [campaign.data, flow.data])
}

type GroupOption = { id: string; slug: string; name: string }

type Loaded = {
  id: string | null
  channelId: string
  name: string
  subject: string
  preheader: string
  blocks: EmailBlock[]
  styles: EmailStyles
  groupId: string | null
}

// The editable document - the unit undo/redo snapshots
type Doc = {
  name: string
  subject: string
  preheader: string
  groupId: string | null
  items: CanvasItem[]
  styles: EmailStyles
}

function itemsFromBlocks(blocks: EmailBlock[]): CanvasItem[] {
  return blocks.map((b) => ({ id: nextId(), block: b }))
}

export function EmailLibraryDetail() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [params, setParams] = useSearchParams()
  const { effectiveChannelId, isChannel, channels } = useConfigScope()
  const isNew = !id || id === 'new'
  // The language being edited - '' is the email itself, a locale its version in that language
  const lang = isNew ? '' : (params.get('lang') ?? '')

  const { data, loading, error, refetch } = useQuery(LibraryEmailDocument, { variables: { id: id ?? '' }, skip: isNew, fetchPolicy: 'cache-and-network' })
  const translationsQuery = useQuery(EmailTranslationsDocument, { variables: { emailId: id ?? '' }, skip: isNew, fetchPolicy: 'cache-and-network' })
  const translations = translationsQuery.data?.emailTranslations ?? []
  const { data: groupsData } = useQuery(EmailGroupsDocument, { fetchPolicy: 'cache-and-network' })
  const groups: GroupOption[] = groupsData?.emailGroups ?? []
  // New emails default to the Marketing group; the operator can retag as needed
  const defaultGroupId = groups.find((g) => g.slug === 'marketing')?.id ?? null

  if (isNew && !isChannel) {
    return (
      <DetailPageLayout>
        <DetailPageHeader title="New email" backTo="/emails" />
        <div className="p-6">
          <Alert>
            <AlertTitle>Pick a channel</AlertTitle>
            <AlertDescription>Select a channel in the header before building an email.</AlertDescription>
          </Alert>
        </div>
      </DetailPageLayout>
    )
  }
  // A failed load or a missing record used to leave the editor's header over an empty body
  if (!isNew && !data?.libraryEmail && error) {
    return <LoadError title="Couldn't load this email" message={error.message} onRetry={() => void refetch()} retrying={loading} />
  }
  if (!isNew && !data?.libraryEmail && !loading) return <NotFound />
  // A language's version has to be known before the canvas opens, or edits made while it loads are lost to the remount
  if (!isNew && (!data?.libraryEmail || (lang && !translationsQuery.data && !translationsQuery.error))) {
    return (
      <DetailPageLayout>
        <DetailPageHeader title="Email" backTo="/emails" />
        <div className="p-6">{loading ? <LoadingState /> : null}</div>
      </DetailPageLayout>
    )
  }

  const initial: Loaded = isNew
    ? { id: null, channelId: effectiveChannelId ?? '', name: '', subject: '', preheader: '', blocks: [], styles: {}, groupId: defaultGroupId }
    : {
        id: data!.libraryEmail!.id,
        channelId: data!.libraryEmail!.channelId,
        name: data!.libraryEmail!.name,
        subject: data!.libraryEmail!.subject,
        preheader: data!.libraryEmail!.preheader ?? '',
        blocks: (data!.libraryEmail!.blocks as EmailBlock[] | null) ?? [],
        styles: (data!.libraryEmail!.styles as EmailStyles | null) ?? {},
        groupId: data!.libraryEmail!.groupId ?? null,
      }

  // A language with no version yet starts from a copy of the email, to translate in place
  const translation = lang ? translations.find((t) => t.locale === lang) : undefined
  const content: Loaded = translation
    ? {
        ...initial,
        subject: translation.subject,
        preheader: translation.preheader,
        blocks: (translation.blocks as EmailBlock[] | null) ?? [],
        styles: (translation.styles as EmailStyles | null) ?? {},
      }
    : initial
  const channel = channels.find((c) => c.id === initial.channelId)
  const emailUpdatedAt = data?.libraryEmail?.updatedAt ?? ''
  // A version saved before its channel dropped the language still exists, so it stays reachable to edit or delete
  const languages: Languages | undefined =
    isNew || !channel
      ? undefined
      : {
          current: lang,
          defaultLocale: channel.defaultLocale,
          others: [...new Set([...channel.locales.map((l) => l.code), ...translations.map((t) => t.locale)])].filter(
            (c) => c !== channel.defaultLocale,
          ),
          translated: translations.map((t) => t.locale),
          outdated: translations.filter((t) => emailUpdatedAt !== '' && t.updatedAt < emailUpdatedAt).map((t) => t.locale),
          offChannel: translations.map((t) => t.locale).filter((c) => !channel.locales.some((l) => l.code === c)),
          exists: translation !== undefined,
          onSwitch: (code) => setParams(code ? { lang: code } : {}),
          onChanged: () => void translationsQuery.refetch(),
        }

  // Remount the editor once the default group resolves (new) or per email and language (edit)
  // so the initial group is applied without setState-in-effect
  const editorKey = isNew
    ? `new-${defaultGroupId ?? 'none'}`
    : `${initial.id ?? 'new'}:${lang}:${translation?.updatedAt ?? ''}`

  return (
    <Editor
      key={editorKey}
      initial={content}
      groups={groups}
      locale={lang}
      languages={languages}
      onCreated={(nid) => void duringCommit(() => navigate(`/emails/${nid}`))}
    />
  )
}

// The languages an email can be written in - its channel's locales, the default being the email itself
type Languages = {
  current: string
  defaultLocale: string
  others: string[]
  translated: string[]
  outdated: string[] // versions saved before the email last changed
  offChannel: string[] // versions of a language the channel no longer sells in
  exists: boolean
  onSwitch: (locale: string) => void
  onChanged: () => void
}

// Undo/redo state - present doc plus bounded past/future stacks
type Hist = { past: Doc[]; present: Doc; future: Doc[] }
const HISTORY_LIMIT = 100

function Editor({
  initial,
  groups,
  locale,
  languages,
  onCreated,
}: {
  initial: Loaded
  groups: GroupOption[]
  // The language being edited, straight from the URL - '' is the email itself. Never read off languages, which is
  // missing while the channel isn't in scope, so a version could otherwise save over the email
  locale: string
  languages?: Languages
  onCreated: (id: string) => void
}) {
  const mergeTags = useMergeTags()
  const [hist, setHist] = useState<Hist>(() => ({
    past: [],
    present: {
      name: initial.name,
      subject: initial.subject,
      preheader: initial.preheader,
      groupId: initial.groupId,
      items: itemsFromBlocks(initial.blocks),
      styles: initial.styles,
    },
    future: [],
  }))
  const doc = hist.present
  // Coalesce consecutive edits of the same field (e.g. typing) into one undo step
  const coalesceKey = useRef<string | null>(null)

  function commit(update: (d: Doc) => Doc, coalesce?: string) {
    setHist((h) => {
      const next = update(h.present)
      if (next === h.present) return h
      if (coalesce && coalesce === coalesceKey.current) {
        return { ...h, present: next, future: [] }
      }
      coalesceKey.current = coalesce ?? null
      return { past: [...h.past, h.present].slice(-HISTORY_LIMIT), present: next, future: [] }
    })
  }
  function undo() {
    coalesceKey.current = null
    setHist((h) => (h.past.length === 0 ? h : { past: h.past.slice(0, -1), present: h.past[h.past.length - 1], future: [h.present, ...h.future] }))
  }
  function redo() {
    coalesceKey.current = null
    setHist((h) => (h.future.length === 0 ? h : { past: [...h.past, h.present], present: h.future[0], future: h.future.slice(1) }))
  }
  // Replace the whole document in one history step (restore from a revision)
  function replaceDoc(next: Doc) {
    coalesceKey.current = null
    setHist((h) => ({ past: [...h.past, h.present].slice(-HISTORY_LIMIT), present: next, future: [] }))
  }

  const [sel, setSel] = useState<string | null>(null)
  const [emailId, setEmailId] = useState<string | null>(initial.id)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [previewOpen, setPreviewOpen] = useState(false)
  const [historyOpen, setHistoryOpen] = useState(false)
  const [saveBlockFor, setSaveBlockFor] = useState<EmailBlock | null>(null)

  const blocks = useMemo(() => doc.items.map((i) => i.block), [doc.items])
  const { data: blocksData } = useQuery(EmailBlocksDocument, { variables: { channelId: initial.channelId } })
  const components: BlockComponent[] = useMemo(() => (blocksData?.emailBlocks ?? []).map(toComponent), [blocksData])
  const compMap = useMemo(() => componentMap(components), [components])
  const snippetList = components.map((c) => ({ slug: c.slug, name: c.name }))

  const { data: aiStatusData } = useQuery(EmailsAiStatusDocument, { fetchPolicy: 'cache-first' })
  const aiEnabled = aiStatusData?.aiStatus.enabled ?? false

  const selectedBlock = findBlock(doc.items, sel)
  const selectedIsRow = !!selectedBlock && selectedBlock.type === 'row' && !!sel && !sel.includes('::')

  function makeBlock(type: string): EmailBlock {
    const c = compMap[type]
    return c ? newBlockRef(c) : newBlock(type)
  }
  function setItems(updater: (items: CanvasItem[]) => CanvasItem[], coalesce?: string) {
    commit((d) => ({ ...d, items: updater(d.items) }), coalesce)
  }
  function deleteSel(target: string) {
    setItems((prev) => removeBlock(prev, target))
    setSel((cur) => (cur === target || (cur && cur.startsWith(`${target}::`)) ? null : cur))
  }
  function duplicateSel(target: string) {
    setItems((prev) => duplicateBlock(prev, target, nextId))
  }
  function editBlock(target: string, next: EmailBlock) {
    setItems((prev) => replaceBlock(prev, target, next), `block:${target}`)
  }
  function updateSelected(next: EmailBlock) {
    if (sel) editBlock(sel, next)
  }
  function onMoveChild(rowId: string, col: number, idx: number, dir: -1 | 1) {
    setItems((prev) => moveChild(prev, rowId, col, idx, dir))
  }
  function insertAt(index: number, type: string) {
    const item = { id: nextId(), block: makeBlock(type) }
    setItems((prev) => {
      const next = [...prev]
      next.splice(Math.max(0, Math.min(index, prev.length)), 0, item)
      return next
    })
    setSel(item.id)
  }
  // A palette click lands after the selected block - after its row when it sits in a column - or at the end
  function addBlock(type: string) {
    const at = sel ? doc.items.findIndex((i) => i.id === parseSel(sel).id) : -1
    insertAt(at === -1 ? doc.items.length : at + 1, type)
  }
  function startFrom(starter: EmailBlock[]) {
    setItems(() => itemsFromBlocks(structuredClone(starter)))
    setSel(null)
  }
  function moveSel(target: string, dir: -1 | 1) {
    const p = parseSel(target)
    if (p.col !== undefined) {
      const count = findBlock(doc.items, p.id)?.columns?.[p.col]?.blocks?.length ?? 0
      const to = (p.idx ?? 0) + dir
      if (to < 0 || to >= count) return
      onMoveChild(p.id, p.col, p.idx ?? 0, dir)
      setSel(childSel(p.id, p.col, to))
      return
    }
    setItems((prev) => {
      const from = prev.findIndex((i) => i.id === p.id)
      const to = from + dir
      if (from === -1 || to < 0 || to >= prev.length) return prev
      return arrayMove(prev, from, to)
    })
  }
  useCanvasShortcuts({
    sel,
    order: doc.items.map((i) => i.id),
    select: setSel,
    remove: deleteSel,
    duplicate: duplicateSel,
    move: moveSel,
    undo,
    redo,
  })
  const { dropBefore, dndProps } = useCanvasDnd({
    items: doc.items,
    setItems: (update) => setItems(update),
    makeBlock,
    canNest: (type) => type !== 'row' && type !== 'ref' && !compMap[type],
    nextId,
    onPlaced: (placed) => placed && setSel(placed),
  })

  // Pre-send checklist (deliverability + quality warnings) - only for a saved
  // email - Refetched after each successful save so the list reflects edits
  const { data: preflightData, refetch: refetchPreflight } = useQuery(EmailPreflightDocument, {
    variables: { id: emailId ?? '' },
    skip: !emailId,
    fetchPolicy: 'cache-and-network',
  })
  const warnings = preflightData?.emailPreflight ?? []

  const [saveEmail] = useMutation(SaveLibraryEmailDocument)
  const [saveTranslation] = useMutation(SaveEmailTranslationDocument)
  const [deleteTranslation] = useMutation(DeleteEmailTranslationDocument)

  async function onDeleteTranslation() {
    if (!emailId || !locale) return
    setError(null)
    try {
      await deleteTranslation({ variables: { emailId, locale } })
      languages?.onChanged()
      languages?.onSwitch('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Delete failed.')
    }
  }
  // hist.present is the live design - the undo stacks around it are not unsaved work
  useUnsavedFields([hist.present])
  const [savedDoc, setSavedDoc] = useState(() => JSON.stringify(hist.present))
  const dirty = JSON.stringify(doc) !== savedDoc

  async function onSave() {
    if (locale && emailId) {
      setError(null)
      setSaving(true)
      try {
        await saveTranslation({
          variables: { input: { emailId, locale, subject: doc.subject, preheader: doc.preheader, blocks, styles: doc.styles } },
        })
        setSavedDoc(JSON.stringify(doc))
        languages?.onChanged()
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Save failed.')
      } finally {
        setSaving(false)
      }
      return
    }
    if (!doc.name.trim()) {
      setError('Give the email a name.')
      return
    }
    setError(null)
    setSaving(true)
    try {
      const res = await saveEmail({
        variables: { input: { id: emailId, channelId: initial.channelId, name: doc.name.trim(), subject: doc.subject, preheader: doc.preheader, blocks, styles: doc.styles, groupId: doc.groupId } },
      })
      const nid = res.data?.saveLibraryEmail?.id ?? null
      setSavedDoc(JSON.stringify(doc))
      if (nid && !emailId) {
        setEmailId(nid)
        onCreated(nid)
      } else if (emailId) {
        // Re-run the checklist against the just-saved content
        void refetchPreflight()
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Save failed.')
    } finally {
      setSaving(false)
    }
  }

  // AI copy assist (only wired when AI is configured)
  const [rewriteText] = useMutation(RewriteEmailTextDocument)
  const aiRewrite: AiRewriteFn | undefined = aiEnabled
    ? async (text, instruction) => {
        const res = await rewriteText({ variables: { text, instruction } })
        return res.data?.rewriteEmailText ?? text
      }
    : undefined

  // Restore a prior version → load its content into the editor as one undo step
  const [restoreRevision] = useMutation(RestoreEmailRevisionDocument)
  async function onRestore(revisionId: string) {
    const res = await restoreRevision({ variables: { revisionId } })
    const e = res.data?.restoreEmailRevision
    if (!e) return
    replaceDoc({
      name: e.name,
      subject: e.subject,
      preheader: e.preheader ?? '',
      groupId: e.groupId ?? null,
      items: itemsFromBlocks((e.blocks as EmailBlock[] | null) ?? []),
      styles: (e.styles as EmailStyles | null) ?? {},
    })
    setSel(null)
    setHistoryOpen(false)
    void refetchPreflight()
  }

  // Pinned action bar for the right sidebar
  const actionsFooter = (
    <div className="space-y-2">
      {error && (
        <Alert variant="destructive">
          <AlertTitle>Error</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <div className="flex items-center gap-1">
        <Button type="button" variant="outline" size="icon" className="size-8 shrink-0" aria-label="Undo" disabled={hist.past.length === 0} onClick={undo}>
          <Undo2Icon className="size-4" />
        </Button>
        <Button type="button" variant="outline" size="icon" className="size-8 shrink-0" aria-label="Redo" disabled={hist.future.length === 0} onClick={redo}>
          <Redo2Icon className="size-4" />
        </Button>
        {emailId && !locale && (
          <Button type="button" variant="outline" size="icon" className="size-8 shrink-0" aria-label="Version history" onClick={() => setHistoryOpen(true)}>
            <HistoryIcon className="size-4" />
          </Button>
        )}
        <Button type="button" variant="outline" size="sm" className="flex-1" onClick={() => setPreviewOpen(true)}>
          Preview
        </Button>
        <Button type="button" size="sm" className="flex-1" disabled={saving} onClick={onSave}>
          {saving ? 'Saving…' : 'Save'}
        </Button>
      </div>
    </div>
  )

  return (
    <DndContext {...dndProps}>
      <DetailPageLayout
        className={cn('@container/editor', PANEL_BACKDROP)}
        leftSidebar={<BlockPalette components={components} onAdd={addBlock} />}
        rightSidebar={
          selectedIsRow && selectedBlock ? (
            <aside className={cn(FLOATING_PANEL, 'ml-0 w-72 @min-[1200px]/editor:w-80')}>
              <div className="border-b px-4 py-3.5">
                <h2 className={PANEL_TITLE}>Columns</h2>
              </div>
              <div className="flex-1 overflow-y-auto p-4">
                <RowInspector block={selectedBlock} onChange={updateSelected} />
              </div>
              <div className="shrink-0 border-t p-3">{actionsFooter}</div>
            </aside>
          ) : (
            <BlockInspector
              block={selectedBlock}
              blockKey={sel}
              inlineText
              onChangeBlock={updateSelected}
              subject={doc.subject}
              onSubjectChange={(s) => commit((d) => ({ ...d, subject: s }), 'subject')}
              mergeTags={mergeTags}
              snippets={snippetList}
              components={compMap}
              showSubject
              aiRewrite={aiRewrite}
              styleControls={
                selectedBlock ? (
                  <>
                    <BlockStyleControls block={selectedBlock} onChange={updateSelected} theme={doc.styles} />
                    {sel && !sel.includes('::') && selectedBlock.type !== 'ref' && (
                      <Button type="button" variant="outline" size="sm" className="w-full" onClick={() => setSaveBlockFor(selectedBlock)}>
                        Save as reusable block
                      </Button>
                    )}
                  </>
                ) : null
              }
              emailSettings={
                <div className="grid gap-4">
                  {languages && (languages.others.length > 0 || languages.current) ? (
                    <LanguagePicker languages={languages} dirty={dirty} onDelete={onDeleteTranslation} />
                  ) : null}
                  {aiEnabled && (
                    <SubjectAssist
                      subject={doc.subject}
                      name={doc.name}
                      preheader={doc.preheader}
                      blocks={blocks}
                      onPick={(s) => commit((d) => ({ ...d, subject: s }))}
                    />
                  )}
                  <div className="grid gap-1.5">
                    <div className="flex items-center justify-between">
                      <Label htmlFor="email-preheader">Preheader</Label>
                      <MergeTagMenu
                        tags={mergeTags}
                        onPick={(tag) => {
                          const next = insertAtCaret('email-preheader', doc.preheader, tag)
                          commit((d) => ({ ...d, preheader: next.value }))
                          next.restore()
                        }}
                      />
                    </div>
                    <Input
                      id="email-preheader"
                      value={doc.preheader}
                      onChange={(e) => commit((d) => ({ ...d, preheader: e.target.value }), 'preheader')}
                      placeholder="Inbox-preview snippet"
                    />
                    <p className="text-xs text-muted-foreground">Shown after the subject in the inbox preview.</p>
                  </div>
                  <div className="grid gap-1.5">
                    <Label htmlFor="email-name">Email name</Label>
                    <Input
                      id="email-name"
                      value={doc.name}
                      disabled={locale !== ''}
                      onChange={(e) => commit((d) => ({ ...d, name: e.target.value }), 'name')}
                      placeholder="Welcome hero"
                    />
                    {locale ? <p className="text-xs text-muted-foreground">The name and group are shared by every language.</p> : null}
                  </div>
                  <div className="grid gap-1.5">
                    <Label htmlFor="email-group">Group</Label>
                    <Select
                      value={doc.groupId ?? NO_GROUP}
                      disabled={locale !== ''}
                      onValueChange={(v) => commit((d) => ({ ...d, groupId: v === NO_GROUP ? null : v }))}
                    >
                      <SelectTrigger id="email-group">
                        <SelectValue placeholder="No group" />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value={NO_GROUP}>No group</SelectItem>
                        {groups.map((g) => (
                          <SelectItem key={g.id} value={g.id}>
                            {g.name}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  {emailId && !locale && <PreflightPanel warnings={warnings} />}
                </div>
              }
              themeSettings={<GlobalStyleControls value={doc.styles} onChange={(s) => commit((d) => ({ ...d, styles: s }), 'styles')} />}
              footer={actionsFooter}
            />
          )
        }
      >
        <EmailCanvas
          items={doc.items}
          sel={sel}
          onSelect={setSel}
          onDelete={deleteSel}
          onDuplicate={duplicateSel}
          onMoveChild={onMoveChild}
          onEdit={editBlock}
          onInsertAt={insertAt}
          onStart={startFrom}
          mergeTags={mergeTags}
          canAdd
          toolbar={
            <>
              <EmailSizeMeter channelId={initial.channelId} subject={doc.subject} preheader={doc.preheader} blocks={blocks} styles={doc.styles} />
              <ShortcutsHelp />
            </>
          }
          theme={doc.styles}
          components={compMap}
          subject={doc.subject}
          emailFrame
          dropBefore={dropBefore}
        />
      </DetailPageLayout>

      <PreviewDialog
        open={previewOpen}
        onOpenChange={setPreviewOpen}
        channelId={initial.channelId}
        subject={doc.subject}
        preheader={doc.preheader}
        blocks={blocks}
        styles={doc.styles}
      />

      <HistoryDialog open={historyOpen} onOpenChange={setHistoryOpen} emailId={emailId} onRestore={onRestore} />

      <SaveAsBlockDialog block={saveBlockFor} channelId={initial.channelId} onClose={() => setSaveBlockFor(null)} />
    </DndContext>
  )
}

// LanguagePicker switches the canvas between the email and its version in each of the channel's other languages. A
// shopper whose storefront language has a version gets it, everyone else the email itself
function LanguagePicker({ languages, dirty, onDelete }: { languages: Languages; dirty: boolean; onDelete: () => void }) {
  const { current, defaultLocale, others, translated, outdated, offChannel, exists } = languages
  const [confirm, confirmDialog] = useConfirmDialog()
  // Switching reloads the canvas, and the router lets a same-page change through without its unsaved-changes prompt
  function choose(value: string) {
    const code = value === DEFAULT_LANGUAGE ? '' : value
    if (code === current) return
    if (!dirty) return languages.onSwitch(code)
    confirm({
      title: 'Discard your changes?',
      description: `Switching to ${languageName(code || defaultLocale)} drops what you haven't saved here.`,
      confirmLabel: 'Discard and switch',
      destructive: true,
      onConfirm: () => languages.onSwitch(code),
    })
  }
  return (
    <div className="grid gap-1.5">
      <Label htmlFor="email-language">Language</Label>
      <Select value={current || DEFAULT_LANGUAGE} onValueChange={choose}>
        <SelectTrigger id="email-language">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={DEFAULT_LANGUAGE}>{languageName(defaultLocale)} (default)</SelectItem>
          {others.map((code) => (
            <SelectItem key={code} value={code}>
              {languageName(code)}
              {!translated.includes(code)
                ? ' - not translated yet'
                : offChannel.includes(code)
                  ? ' - no longer on the channel'
                  : outdated.includes(code)
                    ? ' - older than the default'
                    : ''}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <p className="text-xs text-muted-foreground">
        {current === ''
          ? 'Shoppers in a language with its own version get that one, everyone else this email.'
          : exists
            ? `Shoppers browsing in ${languageName(current)} get this version.${outdated.includes(current) ? ' The default changed after it was saved, so check it still matches.' : ''}`
            : `Starts as a copy of the default - translate it and save, and shoppers browsing in ${languageName(current)} get it.`}
      </p>
      {current !== '' && exists ? (
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="justify-self-start text-destructive"
          onClick={() =>
            confirm({
              title: `Delete the ${current} version?`,
              description: `Shoppers browsing in ${current} get the default email again.`,
              confirmLabel: 'Delete version',
              destructive: true,
              onConfirm: onDelete,
            })
          }
        >
          Delete the {current} version
        </Button>
      ) : null}
      {confirmDialog}
    </div>
  )
}

const DEFAULT_LANGUAGE = '__default__'

// "German (de)" for a locale code, or the code alone when the browser can't name it
function languageName(code: string): string {
  try {
    const name = new Intl.DisplayNames(['en'], { type: 'language' }).of(code)
    return name && name !== code ? `${name} (${code})` : code
  } catch {
    return code
  }
}

// PreflightPanel renders the pre-send checklist: warn findings in a destructive
// tone, info findings muted - Empty = all checks passed
function PreflightPanel({ warnings }: { warnings: Array<{ severity: string; message: string }> }) {
  return (
    <div className="grid gap-1.5">
      <Label>Pre-send checklist</Label>
      {warnings.length === 0 ? (
        <p className="rounded-md border bg-muted/30 px-2.5 py-2 text-xs text-muted-foreground">All checks passed ✓</p>
      ) : (
        <ul className="grid gap-1.5">
          {warnings.map((w, i) => (
            <li
              key={i}
              className={
                w.severity === 'warn'
                  ? 'rounded-md border border-warning/30 bg-warning-soft px-2.5 py-1.5 text-xs text-warning'
                  : 'rounded-md border bg-muted/30 px-2.5 py-1.5 text-xs text-muted-foreground'
              }
            >
              {w.message}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

// SubjectAssist offers AI subject-line suggestions built from the email's name,
// preheader and copy - Clicking a suggestion sets the subject
function SubjectAssist({
  subject,
  name,
  preheader,
  blocks,
  onPick,
}: {
  subject: string
  name: string
  preheader: string
  blocks: EmailBlock[]
  onPick: (s: string) => void
}) {
  const [suggest] = useMutation(SuggestEmailSubjectsDocument)
  const [busy, setBusy] = useState(false)
  const [ideas, setIdeas] = useState<string[]>([])
  const [err, setErr] = useState<string | null>(null)

  function buildContext(): string {
    const copy = blocks
      .flatMap((b) => [b.text, b.content, b.label])
      .filter((s): s is string => !!s && s.trim() !== '')
      .join(' ')
    return [name && `Email: ${name}`, subject && `Current subject: ${subject}`, preheader && `Preheader: ${preheader}`, copy && `Body: ${copy}`]
      .filter(Boolean)
      .join('\n')
      .slice(0, 2000)
  }

  async function run() {
    setBusy(true)
    setErr(null)
    try {
      const res = await suggest({ variables: { context: buildContext(), count: 5 } })
      setIdeas(res.data?.suggestEmailSubjects ?? [])
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Could not generate suggestions.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="grid gap-2 rounded-md border bg-muted/20 p-2.5">
      <div className="flex items-center justify-between">
        <Label className="text-xs">Subject ideas</Label>
        <Button type="button" variant="outline" size="sm" className="h-7" disabled={busy} onClick={run}>
          <SparklesIcon className="mr-1 size-3.5" />
          {busy ? 'Thinking…' : 'Suggest'}
        </Button>
      </div>
      {err && <p className="text-xs text-destructive">{err}</p>}
      {ideas.length > 0 && (
        <ul className="grid gap-1">
          {ideas.map((idea, i) => (
            <li key={i}>
              <button
                type="button"
                onClick={() => onPick(idea)}
                className="w-full rounded border bg-background px-2 py-1 text-left text-xs hover:bg-accent hover:text-accent-foreground"
              >
                {idea}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

// HistoryDialog lists an email's saved versions and restores one
function HistoryDialog({
  open,
  onOpenChange,
  emailId,
  onRestore,
}: {
  open: boolean
  onOpenChange: (o: boolean) => void
  emailId: string | null
  onRestore: (revisionId: string) => Promise<void>
}) {
  const { data, loading } = useQuery(EmailRevisionsDocument, {
    variables: { id: emailId ?? '' },
    skip: !open || !emailId,
    fetchPolicy: 'cache-and-network',
  })
  const [busyId, setBusyId] = useState<string | null>(null)
  const revisions = data?.emailRevisions ?? []

  async function restore(id: string) {
    setBusyId(id)
    try {
      await onRestore(id)
    } finally {
      setBusyId(null)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Version history</DialogTitle>
          <DialogDescription>Each save records a version. Restore one to bring its content back (recorded as a new version).</DialogDescription>
        </DialogHeader>
        {loading && revisions.length === 0 ? (
          <LoadingState />
        ) : revisions.length === 0 ? (
          <p className="py-6 text-center text-sm text-muted-foreground">No versions yet.</p>
        ) : (
          <ul className="max-h-96 divide-y overflow-y-auto">
            {revisions.map((r, i) => (
              <li key={r.id} className="flex items-center justify-between gap-3 py-2.5">
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">
                    {r.subject || <span className="text-muted-foreground">(no subject)</span>}
                    {i === 0 && <span className="ml-2 rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">latest</span>}
                  </p>
                  <p className="text-xs text-muted-foreground">{formatWhen(r.createdAt)}</p>
                </div>
                <Button type="button" variant="outline" size="sm" disabled={busyId !== null} onClick={() => void restore(r.id)}>
                  {busyId === r.id ? 'Restoring…' : 'Restore'}
                </Button>
              </li>
            ))}
          </ul>
        )}
      </DialogContent>
    </Dialog>
  )
}

function formatWhen(v: unknown): string {
  const d = new Date(String(v))
  return Number.isNaN(d.getTime()) ? String(v) : d.toLocaleString()
}

// SaveAsBlockDialog saves the selected block as a reusable library block
function SaveAsBlockDialog({
  block,
  channelId,
  onClose,
}: {
  block: EmailBlock | null
  channelId: string
  onClose: () => void
}) {
  const [name, setName] = useState('')
  const [saveBlock] = useMutation(SaveEmailBlockDocument)
  const [saving, setSaving] = useState(false)
  const [err, setErr] = useState<string | null>(null)

  const slug = name
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')

  async function save() {
    if (!slug) {
      setErr('Give the block a name.')
      return
    }
    setErr(null)
    setSaving(true)
    try {
      await saveBlock({ variables: { input: { channelId, slug, name: name.trim(), blocks: block ? [block] : [] } } })
      onClose()
      setName('')
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Save failed.')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={!!block} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Save as reusable block</DialogTitle>
          <DialogDescription>Adds this block to the channel’s reusable library so you can drop it into other emails.</DialogDescription>
        </DialogHeader>
        <div className="grid gap-3">
          {block && <p className="rounded-md border bg-muted/30 px-2.5 py-1.5 text-xs text-muted-foreground">{shortPreview(block) || block.type}</p>}
          <div className="grid gap-1.5">
            <Label htmlFor="block-name">Name</Label>
            <Input id="block-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="Promo hero" autoFocus />
            {slug && <p className="font-mono text-xs text-muted-foreground">{slug}</p>}
          </div>
          {err && <p className="text-xs text-destructive">{err}</p>}
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button type="button" disabled={saving} onClick={save}>
            {saving ? 'Saving…' : 'Save block'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
