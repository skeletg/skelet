import { act, fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { MediaPickerProps } from './picker/MediaPickerDialog'
import { MediaImageInput } from './MediaImageInput'

// What this pins: the image field uploads into its own folder and stores a library ref, says why an upload failed, opens
// the picker on its current library image with uploads going to the same folder, and a pick carries the asset's alt text

const picker = vi.hoisted(() => ({ props: null as MediaPickerProps | null }))
vi.mock('./MediaAssetPicker', () => ({
  MediaAssetPicker: (p: MediaPickerProps) => {
    picker.props = p
    return null
  },
}))
const upload = vi.hoisted(() => ({ path: '', fn: vi.fn() }))
vi.mock('./useImageUpload', () => ({
  useImageUpload: (path: string) => {
    upload.path = path
    return upload.fn
  },
}))

const fileInput = (c: HTMLElement) => c.querySelector('input[type="file"]') as HTMLInputElement
const ref = { type: 'asset' as const, assetId: 'm1', url: 'https://cdn/m1.png', alt: '' }

describe('MediaImageInput', () => {
  it('uploads into its folder and stores a library ref', async () => {
    upload.fn.mockResolvedValue({ assetId: 'm9', url: 'https://cdn/m9.png' })
    const onChange = vi.fn()
    const { container } = render(<MediaImageInput value={undefined} onChange={onChange} path="settings/brand" />)
    const file = new File(['x'], 'logo.png', { type: 'image/png' })
    await act(async () => {
      fireEvent.change(fileInput(container), { target: { files: [file] } })
    })
    expect(upload.path).toBe('settings/brand')
    expect(upload.fn).toHaveBeenCalledWith(file)
    expect(onChange).toHaveBeenCalledWith({ type: 'asset', assetId: 'm9', url: 'https://cdn/m9.png', alt: '' })
  })

  it('says why an upload failed and keeps the value', async () => {
    upload.fn.mockRejectedValue(new Error('Only image files can be uploaded.'))
    const onChange = vi.fn()
    const { container } = render(<MediaImageInput value="https://old.png" onChange={onChange} />)
    await act(async () => {
      fireEvent.change(fileInput(container), { target: { files: [new File(['x'], 'a.txt', { type: 'text/plain' })] } })
    })
    expect(screen.getByText('Only image files can be uploaded.')).toBeDefined()
    expect(onChange).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: /Upload/ })).toHaveProperty('disabled', false)
  })

  it('opens the picker on its library image, uploading to the same folder, and keeps the alt text of a pick', () => {
    const onChange = vi.fn()
    render(<MediaImageInput value={ref} onChange={onChange} path="cms/blog" />)
    fireEvent.click(screen.getByRole('button', { name: /Library/ }))
    expect(picker.props).toMatchObject({ open: true, selectedIds: ['m1'], uploadPath: 'cms/blog' })
    act(() => picker.props?.onSelect?.({ id: 'm2', url: 'https://cdn/m2.png', altText: 'A red mug' } as never))
    expect(onChange).toHaveBeenCalledWith({ type: 'asset', assetId: 'm2', url: 'https://cdn/m2.png', alt: 'A red mug' })
  })

  it('opens the picker with nothing picked for a pasted url', () => {
    render(<MediaImageInput value="https://elsewhere/x.png" onChange={() => {}} />)
    fireEvent.click(screen.getByRole('button', { name: /Library/ }))
    expect(picker.props).toMatchObject({ selectedIds: [], uploadPath: 'cms/pages' })
  })

  it('edits the alt text of a library image and removes it', () => {
    const onChange = vi.fn()
    render(<MediaImageInput value={ref} onChange={onChange} />)
    fireEvent.change(screen.getByPlaceholderText(/Alt text/), { target: { value: 'Shop front' } })
    expect(onChange).toHaveBeenLastCalledWith({ ...ref, alt: 'Shop front' })
    fireEvent.click(screen.getByRole('button', { name: 'Remove image' }))
    expect(onChange).toHaveBeenLastCalledWith(undefined)
  })
})
