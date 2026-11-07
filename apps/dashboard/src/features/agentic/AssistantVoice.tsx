import { useRef } from 'react'
import { Link } from 'react-router-dom'
import { PlusIcon } from 'lucide-react'
import { DetailPageBody, DetailPageHeader, DetailPageLayout, DetailPageSection } from '@/components/layout/detail-page'
import { PickStore } from '@/components/layout/panels'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { LoadingState } from '@/components/ui/loading-state'
import type { ConfigFieldData } from '@/features/settings/ConfigField'
import { useConfigScope } from '@/lib/config-scope'
import { cn } from '@/lib/utils'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'
import { SaveRegistryContext, useSaveRegistry } from '@/shared/saving/saveRegistry'
import { useSaveConfirm } from '@/shared/saving/useSaveConfirm'
import { AuroraStage, Presence, Streamed } from './aurora'
import { FieldState, SegmentedRadio, type Field } from './FieldState'
import { useConfigGroupFields } from './useConfigGroupFields'
import { useStreamed, useTyping } from './useAssistant'

const SECTION = 'sales'
const VOICE_GROUP = 'assistant'
const NAME = 'sales/assistant/name'
const TONE = 'sales/assistant/tone'
const INSTRUCTIONS = 'sales/assistant/instructions'
const SWITCH = 'sales/agents/assistant_enabled'

// One question answered in each tone, so a tone can be heard before it's picked - an example, not the model's words
const ASKED = 'Anything for rainy hikes?'
const SAMPLES: Record<string, string> = {
  friendly: 'Happy to help! These two shells keep the rain out and still breathe on the climb - the lighter one packs into its own pocket.',
  professional: 'Certainly. Both shells are fully waterproof and breathable, and the lighter one packs into its own pocket.',
  playful: 'Rain, meet your match. These two shells shrug off a downpour - and the lighter one folds into its own pocket for the sunny bits.',
  brief: 'Two waterproof shells. The lighter one packs into its pocket.',
}

// Each starts a line the merchant finishes - the kinds of thing that make the most difference to its answers
const STARTERS = [
  { label: 'Sizing advice', line: 'Sizing: ', hint: 'Our jeans run small - suggest a size up.' },
  { label: 'What to suggest first', line: 'Suggest first: ', hint: 'Our own brand, then bundles over single items.' },
  { label: "What the pages don't say", line: 'Good to know: ', hint: 'Gift wrapping is free on orders over 50.' },
  { label: 'What to stay away from', line: 'Never: ', hint: 'Medical claims, or comparing us with other shops.' },
]

// The registry sits here and the drafts one level down, since useConfigDrafts registers through the context this provides
export function AssistantVoice() {
  const registry = useSaveRegistry('Assistant voice')
  const confirm = useSaveConfirm(registry.saveAll)
  const { isChannel, selectedChannel } = useConfigScope()
  return (
    <SaveRegistryContext.Provider value={registry.contextValue}>
      <DetailPageLayout>
        <VoiceBody isDirty={registry.isDirty} saving={confirm.saving} onSave={confirm.open} />
        <SaveConfirmDialog {...confirm.dialogProps} scopeLabel={isChannel ? (selectedChannel?.name ?? 'This store') : 'Every store without its own'} sections={registry.dirtySections} />
      </DetailPageLayout>
    </SaveRegistryContext.Provider>
  )
}

function VoiceBody({ isDirty, saving, onSave }: { isDirty: boolean; saving: boolean; onSave: () => void }) {
  const { isChannel, selectedChannel } = useConfigScope()
  const v = useConfigGroupFields(SECTION, VOICE_GROUP)
  const switchedOn = v.section?.groups.flatMap((g) => g.fields).find((f) => f.path === SWITCH)?.value === 'true'
  const localeLock = v.localeLocked ? `The voice isn't per language - clear the language in the header to change it for ${selectedChannel?.name ?? 'this store'}.` : undefined
  const locked = !v.canEdit || !!localeLock
  const { here, from, resetError } = v
  const name = v.fields.get(NAME)
  const tone = v.fields.get(TONE)
  const instructions = v.fields.get(INSTRUCTIONS)
  const [typing, ping] = useTyping()

  return (
    <>
      <DetailPageHeader
        title="Assistant voice"
        badges={isChannel ? <Badge variant="secondary">{selectedChannel?.name}</Badge> : <Badge variant="outline">Every store</Badge>}
        meta={<span className="text-xs text-muted-foreground">Who shoppers talk to on the storefront, and what it should know about the store</span>}
        actions={
          <Button type="button" size="sm" disabled={!isDirty || saving} onClick={onSave}>
            {saving ? 'Saving…' : 'Save changes'}
          </Button>
        }
      />
      <DetailPageBody>
        {v.error ? (
          <DetailPageSection>
            <Alert variant="destructive">
              <AlertTitle>Couldn't load the assistant's settings</AlertTitle>
              <AlertDescription>{v.error.message}</AlertDescription>
            </Alert>
          </DetailPageSection>
        ) : v.noStore ? (
          <DetailPageSection>
            <PickStore why="The voice is set per store" />
          </DetailPageSection>
        ) : !v.permissionsLoading && !v.canView ? (
          <DetailPageSection>
            <p className="text-sm text-muted-foreground">Reading the assistant's voice needs access to settings {isChannel ? 'on this store' : 'for every store'}.</p>
          </DetailPageSection>
        ) : !name || !tone || !instructions ? (
          <DetailPageSection>
            <LoadingState />
          </DetailPageSection>
        ) : (
          <>
            {isChannel && !switchedOn ? (
              <DetailPageSection>
                <Alert variant="info">
                  <AlertTitle>The assistant is off on this store</AlertTitle>
                  <AlertDescription>
                    <span>
                      What you set here applies once it's on - switch it on under{' '}
                      <Link to="/settings/sales/agents" className="underline">
                        Settings, Sales, AI shopping agents
                      </Link>
                      .
                    </span>
                  </AlertDescription>
                </Alert>
              </DetailPageSection>
            ) : null}
            {resetError || localeLock ? (
              <DetailPageSection>
                <Alert variant={resetError ? 'destructive' : 'default'}>
                  <AlertDescription>{resetError ?? localeLock}</AlertDescription>
                </Alert>
              </DetailPageSection>
            ) : null}
            <DetailPageSection>
              <PersonaStage name={name} tone={tone} locked={locked} here={here} from={from} typing={typing} ping={ping} />
            </DetailPageSection>
            <DetailPageSection
              title="What it should know"
              description="Read before every conversation, after its own rules. It takes what you write about the store as fact, but can't be told to take payments or change what its tools do. Delivery, returns and other policies already come from the store's pages."
            >
              <InstructionsEditor field={instructions} locked={locked} here={here} from={from(instructions.data)} ping={ping} />
            </DetailPageSection>
            {!v.canEdit ? (
              <DetailPageSection>
                <p className="text-sm text-muted-foreground">You can see these, but changing them needs permission to manage settings {isChannel ? 'on this store' : 'for every store'}.</p>
              </DetailPageSection>
            ) : null}
          </>
        )}
      </DetailPageBody>
    </>
  )
}

// The stage is the assistant as a shopper meets it: its presence, the name it goes by, and how it answers - the name is
// edited in place, and every change has it answer the same question again in the tone picked
function PersonaStage({
  name,
  tone,
  locked,
  here,
  from,
  typing,
  ping,
}: {
  name: Field
  tone: Field
  locked: boolean
  here: string
  from: (f: ConfigFieldData) => string
  typing: boolean
  ping: () => void
}) {
  const sample = SAMPLES[tone.value] ?? SAMPLES.friendly
  const reply = useStreamed(sample, `${tone.value}|${name.value.trim()}`)
  const options = tone.data.options ?? []
  const max = name.data.maxLength ?? undefined

  return (
    <AuroraStage>
      <div className="relative grid gap-8 p-6 md:p-8 lg:grid-cols-[minmax(0,1fr)_minmax(0,26rem)] lg:gap-12">
        <div className="min-w-0">
          <div className="flex items-center gap-4">
            <Presence speaking={reply.streaming || typing} />
            <label htmlFor="voice-name" className="sr-only">
              {name.data.label}
            </label>
            <input
              id="voice-name"
              value={name.value}
              onChange={(e) => {
                name.set(e.target.value)
                ping()
              }}
              maxLength={max}
              disabled={locked}
              placeholder="Shopping assistant"
              autoComplete="off"
              spellCheck={false}
              className="min-w-0 flex-1 rounded-md border border-transparent bg-transparent px-2 py-1 text-4xl font-semibold tracking-tight outline-none transition-colors placeholder:text-muted-foreground/50 hover:border-border focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:cursor-not-allowed"
            />
          </div>
          <FieldState field={name} here={here} from={from(name.data)} locked={locked} count={max ? [...name.value].length : undefined} max={max} note="Its name, and the title of its panel on the storefront" />

          <p id="voice-tone" className="mt-7 text-sm font-medium">
            {tone.data.label}
          </p>
          <SegmentedRadio labelledBy="voice-tone" options={options} value={tone.value} onChange={tone.set} disabled={locked} columns={2} className="mt-2 max-w-md" />
          <FieldState field={tone} here={here} from={from(tone.data)} locked={locked} />
        </div>

        <figure className="flex min-w-0 flex-col justify-end gap-3" aria-label="A sample reply in this tone">
          <p className="ml-auto w-fit max-w-[85%] rounded-2xl rounded-br-sm bg-muted px-3.5 py-2 text-sm">{ASKED}</p>
          <div className="flex items-start gap-2.5">
            <Presence size="sm" speaking={reply.streaming} className="mt-0.5" />
            <div className="min-w-0">
              <p className="text-xs font-medium text-muted-foreground">{name.value.trim() || 'Shopping assistant'}</p>
              <p className="mt-0.5 min-h-[4.5rem] text-[15px] leading-relaxed">
                <Streamed {...reply} />
              </p>
            </div>
          </div>
          <figcaption className="text-xs text-muted-foreground">A sample of the tone. The assistant writes its own replies from the store's products and pages.</figcaption>
        </figure>
      </div>
    </AuroraStage>
  )
}

function InstructionsEditor({ field, locked, here, from, ping }: { field: Field; locked: boolean; here: string; from: string; ping: () => void }) {
  const box = useRef<HTMLTextAreaElement>(null)
  const max = field.data.maxLength ?? undefined
  const count = [...field.value].length

  // A starter goes on a line of its own at the end, with the caret after it, ready to be finished
  const start = (line: string) => {
    const before = field.value.replace(/\s+$/, '')
    const next = before ? `${before}\n${line}` : line
    if (max && [...next].length > max) return
    field.set(next)
    requestAnimationFrame(() => {
      const el = box.current
      if (!el) return
      el.focus()
      el.setSelectionRange(next.length, next.length)
    })
  }

  return (
    <div className={cn('rounded-md border border-l-2', field.dirty ? 'border-l-warning' : '')}>
      <div className="flex flex-wrap items-center gap-2 border-b px-4 py-3">
        <span className="mr-1 text-sm text-muted-foreground">Start a line</span>
        {STARTERS.map((s) => (
          <Button key={s.label} type="button" variant="outline" size="sm" disabled={locked} title={`For example: ${s.hint}`} onClick={() => start(s.line)}>
            <PlusIcon className="size-3.5" />
            {s.label}
          </Button>
        ))}
      </div>
      <label htmlFor="voice-instructions" className="sr-only">
        {field.data.label}
      </label>
      <textarea
        id="voice-instructions"
        ref={box}
        value={field.value}
        onChange={(e) => {
          field.set(e.target.value)
          ping()
        }}
        maxLength={max}
        disabled={locked}
        placeholder={'Write to the assistant the way you\'d brief a new colleague on the shop floor.\n\nSizing: our jeans run small - suggest a size up.\nGood to know: gift wrapping is free on orders over 50.'}
        className="block min-h-72 w-full resize-y bg-transparent px-4 py-3 text-[15px] leading-relaxed outline-none placeholder:text-muted-foreground/60 focus-visible:bg-muted/20 disabled:cursor-not-allowed"
      />
      <div className="border-t px-4 py-2">
        <FieldState field={field} here={here} from={from} locked={locked} count={max ? count : undefined} max={max} note="Sent with every message, so a longer brief costs a little more each time" />
      </div>
    </div>
  )
}
