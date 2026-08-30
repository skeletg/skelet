import { ColorPicker } from '@/components/ColorPicker'

// Adapts the shared ColorPicker to the config system - the stored value is a lowercase #rrggbb or '', the shape config.parseAndValidate enforces on write
export function ConfigColorField({
  value,
  onChange,
  disabled,
}: {
  value: string
  onChange: (next: string) => void
  disabled?: boolean
}) {
  return (
    <div className="max-w-xs">
      <ColorPicker value={value} onChange={onChange} disabled={disabled} />
    </div>
  )
}
