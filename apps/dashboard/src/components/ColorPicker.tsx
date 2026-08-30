import { useRef, useState } from 'react'

import { cn } from '@/lib/utils'

// The dashboard's reusable color input - one bordered field with a clickable swatch and a
// hex input, plus an optional row of preset chips - No popover picker: native
// <input type="color"> already opens the OS picker with consistent cross-platform UX
// Value-controlled, and typing only commits on a successful parse, so consumers never see
// malformed intermediate states - empty input is allowed and emits ""

const HEX_PATTERN = /^#[0-9a-f]{6}$/i

// Neutrals plus classic promotional accents, so operators get a sensible badge without
// picking from scratch - override via the `presets` prop
const DEFAULT_PRESETS = [
  '#ffffff', '#000000',
  '#dc2626', // red-600   - classic sale red
  '#f59e0b', // amber-500 - warm warning / clearance
  '#facc15', // yellow-400 - bright "DEAL" colour
  '#10b981', // emerald-500 - eco / fresh
  '#3b82f6', // blue-500   - trust / cool
  '#8b5cf6', // violet-500 - premium
]

export function ColorPicker({
  value,
  onChange,
  presets = DEFAULT_PRESETS,
  className,
  disabled,
  ariaLabel,
}: {
  value: string
  onChange: (next: string) => void
  presets?: string[]
  className?: string
  disabled?: boolean
  ariaLabel?: string
}) {
  // Lets the user type freely without sanitising each keystroke - commits on a successful parse or on blur
  const [draft, setDraft] = useState(value || '')
  // State adjustment during render, per "You Might Not Need an Effect"
  const [prevValue, setPrevValue] = useState(value)
  if (prevValue !== value) {
    setPrevValue(value)
    setDraft(value || '')
  }

  // The native input always needs a valid #RRGGBB, so an empty form value anchors the
  // OS picker on white while the committed value stays empty
  const swatchValue = HEX_PATTERN.test(draft) ? draft : '#ffffff'

  const colorInputRef = useRef<HTMLInputElement>(null)

  function commit(next: string) {
    const trimmed = next.trim().toLowerCase()
    if (trimmed === '') {
      onChange('')
      return
    }
    if (HEX_PATTERN.test(trimmed)) {
      onChange(trimmed)
    }
  }

  return (
    <div className={cn('grid gap-1.5', className)}>
      <div
        className={cn(
          'flex h-9 items-stretch overflow-hidden rounded-md border bg-background transition focus-within:ring-1 focus-within:ring-foreground/20',
          disabled && 'opacity-50',
        )}
      >
        <button
          type="button"
          onClick={() => colorInputRef.current?.click()}
          disabled={disabled}
          className="relative grid w-12 shrink-0 cursor-pointer place-items-center border-r"
          style={{ backgroundColor: swatchValue }}
          aria-label={ariaLabel ? `${ariaLabel} - open color picker` : 'Open color picker'}
        >
          {/* Hidden visually but takes the click - the swatch proxies it so the whole 48x36 area activates */}
          <input
            ref={colorInputRef}
            type="color"
            value={swatchValue}
            onChange={(e) => {
              const v = e.target.value.toLowerCase()
              setDraft(v)
              commit(v)
            }}
            disabled={disabled}
            className="sr-only"
            tabIndex={-1}
            aria-hidden="true"
          />
        </button>
        <input
          type="text"
          value={draft}
          onChange={(e) => {
            const next = e.target.value
            setDraft(next)
            commit(next)
          }}
          onBlur={(e) => {
            // On blur, empty stays empty and valid stays valid - anything else snaps back
            // to the last committed value, so the input can't get stuck in a bad state
            const trimmed = e.target.value.trim().toLowerCase()
            if (trimmed === '' || HEX_PATTERN.test(trimmed)) return
            setDraft(value)
          }}
          disabled={disabled}
          placeholder="#000000"
          spellCheck={false}
          aria-label={ariaLabel}
          className="flex-1 bg-transparent px-3 font-mono text-sm outline-none placeholder:text-muted-foreground/60"
        />
        {value ? (
          <button
            type="button"
            onClick={() => {
              setDraft('')
              onChange('')
            }}
            disabled={disabled}
            className="border-l px-2 text-xs text-muted-foreground transition hover:text-foreground"
            aria-label="Clear color"
          >
            Clear
          </button>
        ) : null}
      </div>

      {presets.length > 0 ? (
        <div className="flex flex-wrap gap-1">
          {presets.map((preset) => {
            const selected = value.toLowerCase() === preset.toLowerCase()
            return (
              <button
                type="button"
                key={preset}
                onClick={() => {
                  setDraft(preset)
                  onChange(preset)
                }}
                disabled={disabled}
                className={cn(
                  'size-6 rounded-md border transition hover:scale-110',
                  selected
                    ? 'ring-2 ring-foreground ring-offset-1 ring-offset-background'
                    : 'border-border',
                )}
                style={{ backgroundColor: preset }}
                aria-label={`Use preset ${preset}`}
                aria-pressed={selected}
              />
            )
          })}
        </div>
      ) : null}
    </div>
  )
}
