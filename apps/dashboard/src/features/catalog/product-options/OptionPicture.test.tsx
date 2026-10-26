import { act, fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { MediaPickerProps } from '@/features/media/picker/MediaPickerDialog'
import { OptionPicture } from './OptionPicture'

// What this pins: option and choice pictures go into option-pictures - never product-options/, which the orphan sweep
// empties - from the picker and from a direct upload alike; the picker opens on the current picture, titled for the
// option, and a pick, an upload and a removal each reach onChange

const picker = vi.hoisted(() => ({ props: null as MediaPickerProps | null }))
vi.mock('@/features/media/MediaAssetPicker', () => ({
  MediaAssetPicker: (p: MediaPickerProps) => {
    picker.props = p
    return null
  },
}))
const upload = vi.hoisted(() => ({ path: '', fn: vi.fn() }))
vi.mock('@/features/media/useImageUpload', () => ({
  useImageUpload: (path: string) => {
    upload.path = path
    return upload.fn
  },
}))

const value = { assetId: 'm1', url: 'https://cdn/box.png', altText: 'A gift box' }
const openMenu = () => fireEvent.pointerDown(screen.getByRole('button', { name: 'Gift box picture' }), { button: 0, ctrlKey: false, pointerType: 'mouse' })

describe('OptionPicture', () => {
  it('picks from the library into option-pictures, opening on the current picture', async () => {
    const onChange = vi.fn()
    render(<OptionPicture value={value} onChange={onChange} label="Gift box" />)
    expect(picker.props).toMatchObject({ open: false, uploadPath: 'option-pictures', title: 'Picture for Gift box', selectedIds: ['m1'] })
    openMenu()
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Choose from the library' }))
    expect(picker.props?.open).toBe(true)
    act(() => picker.props?.onSelect?.({ id: 'm2', url: 'https://cdn/ribbon.png', altText: '' } as never))
    expect(onChange).toHaveBeenCalledWith({ assetId: 'm2', url: 'https://cdn/ribbon.png', altText: '' })
  })

  it('uploads straight into option-pictures', async () => {
    upload.fn.mockResolvedValue({ assetId: 'm3', url: 'https://cdn/new.png' })
    const onChange = vi.fn()
    render(<OptionPicture value={null} onChange={onChange} label="Gift box" />)
    expect(picker.props?.selectedIds).toEqual([])
    const file = new File(['x'], 'new.png', { type: 'image/png' })
    await act(async () => {
      fireEvent.change(screen.getByLabelText('Upload Gift box picture'), { target: { files: [file] } })
    })
    expect(upload.path).toBe('option-pictures')
    expect(upload.fn).toHaveBeenCalledWith(file)
    expect(onChange).toHaveBeenCalledWith({ assetId: 'm3', url: 'https://cdn/new.png' })
  })

  it('says why an upload failed', async () => {
    upload.fn.mockRejectedValue(new Error('Upload failed with HTTP 413.'))
    const onChange = vi.fn()
    render(<OptionPicture value={null} onChange={onChange} label="Gift box" />)
    await act(async () => {
      fireEvent.change(screen.getByLabelText('Upload Gift box picture'), { target: { files: [new File(['x'], 'big.png', { type: 'image/png' })] } })
    })
    expect(screen.getByText('Upload failed with HTTP 413.')).toBeDefined()
    expect(onChange).not.toHaveBeenCalled()
  })

  it('removes the picture, and offers no removal without one', async () => {
    const onChange = vi.fn()
    const { rerender } = render(<OptionPicture value={value} onChange={onChange} label="Gift box" />)
    openMenu()
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Remove the picture' }))
    expect(onChange).toHaveBeenCalledWith(null)
    rerender(<OptionPicture value={null} onChange={onChange} label="Gift box" />)
    openMenu()
    await screen.findByRole('menuitem', { name: 'Upload a picture' })
    expect(screen.queryByRole('menuitem', { name: 'Remove the picture' })).toBeNull()
  })
})
