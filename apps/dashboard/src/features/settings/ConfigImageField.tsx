import { MediaImageInput } from '@/features/media/MediaImageInput'
import { isAssetRef } from '@/features/media/asset-ref'

// Adapts the shared media image field to the config system, which stores one opaque string - the
// resolved PUBLIC URL is what's stored, so every consumer treats it as an ordinary image URL
export function ConfigImageField({
  value,
  onChange,
  disabled,
  path,
}: {
  value: string
  onChange: (next: string) => void
  disabled?: boolean
  path?: string
}) {
  return (
    <MediaImageInput
      value={value || undefined}
      disabled={disabled}
      path={path ? `settings/${path}` : 'settings'}
      onChange={(next) => onChange(!next ? '' : isAssetRef(next) ? next.url : next)}
    />
  )
}
