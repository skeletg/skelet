import { MediaPickerDialog, type MediaPickerProps } from './picker/MediaPickerDialog'

// The name every caller already imports - the picker itself is MediaPickerDialog
export function MediaAssetPicker(props: MediaPickerProps) {
  return <MediaPickerDialog {...props} />
}
