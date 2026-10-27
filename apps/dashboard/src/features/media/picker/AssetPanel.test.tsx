import { fireEvent, render, screen } from '@testing-library/react'
import { MockedProvider } from '@apollo/client/testing/react'
import type { MockLink } from '@apollo/client/testing'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SLOW } from '@/test/page-harness'
import {
  GenerateMediaAltTextDocument,
  MediaAiStatusDocument,
  SetMediaFocalPointDocument,
  UpdateMediaAssetDocument,
} from '../MediaLibrary.gql'
import { AssetPanel } from './AssetPanel'

// What this pins: the panel saves only what changed and only when asked, Undo puts the words back, a click on the
// picture sets the focal point as a 0..1 fraction (and Reset clears it), AI writes alt text into the draft without
// saving it, a different asset brings its own words, and without media.manage it only shows

afterEach(() => vi.unstubAllGlobals())

const asset = (over: object = {}) => ({
  __typename: 'MediaAsset' as const, id: 'a1', path: 'promo/red.png', directoryPath: 'promo', filename: 'red.png', url: 'https://cdn/red.png',
  mimeType: 'image/png', mimeGroup: 'IMAGE' as const, sizeBytes: 2048, width: 400, height: 200, status: 'READY' as const,
  title: 'Red', altText: 'A red mug', caption: '', notes: '', focalX: null, focalY: null, createdAt: '2026-10-01T00:00:00Z', updatedAt: '2026-10-01T00:00:00Z',
  ...over,
})

function rig({ ai = false, canManage = true, mocks = [] as MockLink.MockedResponse[], first = false, value = asset() } = {}) {
  const onChanged = vi.fn()
  const sent: { op: string; vars: Record<string, unknown> }[] = []
  const record = (op: string) => (vars: Record<string, unknown>) => {
    sent.push({ op, vars })
    return true
  }
  // `first` puts the test's own mocks ahead of the stock ones, so they answer instead
  const all: MockLink.MockedResponse[] = [
    ...(first ? mocks : []),
    { request: { query: MediaAiStatusDocument }, result: { data: { aiStatus: { __typename: 'AiStatus', enabled: ai } } }, maxUsageCount: 5 },
    { request: { query: UpdateMediaAssetDocument, variables: record('update') }, result: (v: { input: object }) => ({ data: { updateMediaAsset: asset(v.input) } }), maxUsageCount: 5 },
    { request: { query: SetMediaFocalPointDocument, variables: record('focal') }, result: (v: { x: number | null; y: number | null }) => ({ data: { setMediaFocalPoint: asset({ focalX: v.x, focalY: v.y }) } }), maxUsageCount: 5 },
    { request: { query: GenerateMediaAltTextDocument, variables: record('ai') }, result: { data: { generateMediaAltText: 'A glossy red mug on a shelf' } } },
    ...(first ? [] : mocks),
  ]
  const ui = (a: ReturnType<typeof asset>) => (
    <MemoryRouter>
      <MockedProvider mocks={all}>
        <AssetPanel asset={a} canManage={canManage} onChanged={onChanged} />
      </MockedProvider>
    </MemoryRouter>
  )
  const view = render(ui(value))
  return { onChanged, sent, rerender: (a: ReturnType<typeof asset>) => view.rerender(ui(a)) }
}

const field = (label: string) => screen.getByLabelText(label) as HTMLInputElement

describe('AssetPanel', () => {
  it('saves the edited words and hands back the saved asset', async () => {
    const { sent, onChanged } = rig()
    const save = screen.getByRole('button', { name: 'Save details' })
    expect(save).toHaveProperty('disabled', true)
    fireEvent.change(field('Alt text'), { target: { value: 'A big red mug' } })
    fireEvent.change(field('Caption'), { target: { value: 'Autumn range' } })
    fireEvent.click(save)
    await vi.waitFor(() => expect(onChanged).toHaveBeenCalledWith(expect.objectContaining({ altText: 'A big red mug', caption: 'Autumn range' })), SLOW)
    expect(sent).toEqual([{ op: 'update', vars: { input: { id: 'a1', title: 'Red', altText: 'A big red mug', caption: 'Autumn range' } } }])
  })

  it('puts the words back on Undo', () => {
    rig()
    fireEvent.change(field('Title'), { target: { value: 'Blue' } })
    fireEvent.click(screen.getByRole('button', { name: 'Undo' }))
    expect(field('Title').value).toBe('Red')
    expect(screen.queryByRole('button', { name: 'Undo' })).toBeNull()
    expect(screen.getByRole('button', { name: 'Save details' })).toHaveProperty('disabled', true)
  })

  it('says why a save failed and keeps the draft', async () => {
    const { onChanged } = rig({ mocks: [{ request: { query: UpdateMediaAssetDocument, variables: () => true }, error: new Error('Title is too long') }], first: true })
    fireEvent.change(field('Title'), { target: { value: 'Blue' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save details' }))
    expect(await screen.findByText('Title is too long', undefined, SLOW)).toBeDefined()
    expect(field('Title').value).toBe('Blue')
    expect(onChanged).not.toHaveBeenCalled()
  })

  it('sets the focal point where the picture was clicked', async () => {
    const { sent, onChanged } = rig()
    const img = screen.getByRole('img', { name: 'A red mug' })
    img.getBoundingClientRect = () => ({ left: 100, top: 50, width: 400, height: 200 }) as DOMRect
    fireEvent.click(img, { clientX: 200, clientY: 200 })
    await vi.waitFor(() => expect(onChanged).toHaveBeenCalledWith(expect.objectContaining({ focalX: 0.25, focalY: 0.75 })), SLOW)
    expect(sent).toEqual([{ op: 'focal', vars: { assetId: 'a1', x: 0.25, y: 0.75 } }])
  })

  it('keeps a click past the edge inside the picture', async () => {
    const { sent } = rig()
    const img = screen.getByRole('img', { name: 'A red mug' })
    img.getBoundingClientRect = () => ({ left: 0, top: 0, width: 300, height: 300 }) as DOMRect
    fireEvent.click(img, { clientX: -20, clientY: 100 })
    await vi.waitFor(() => expect(sent).toEqual([{ op: 'focal', vars: { assetId: 'a1', x: 0, y: 0.3333 } }]), SLOW)
  })

  it('resets the focal point to the centre', async () => {
    const { sent } = rig({ value: asset({ focalX: 0.2, focalY: 0.4 }) })
    fireEvent.click(screen.getByRole('button', { name: 'Reset to centre' }))
    await vi.waitFor(() => expect(sent).toEqual([{ op: 'focal', vars: { assetId: 'a1', x: null, y: null } }]), SLOW)
  })

  it('writes alt text with AI into the draft without saving it', async () => {
    const { sent } = rig({ ai: true })
    fireEvent.click(await screen.findByRole('button', { name: 'Write with AI' }, SLOW))
    await vi.waitFor(() => expect(field('Alt text').value).toBe('A glossy red mug on a shelf'), SLOW)
    expect(sent.map((s) => s.op)).toEqual(['ai'])
    expect(screen.getByRole('button', { name: 'Save details' })).toHaveProperty('disabled', false)
  })

  it('offers no AI button when AI is off', async () => {
    rig({ ai: false })
    await new Promise((r) => setTimeout(r, 20))
    expect(screen.queryByRole('button', { name: 'Write with AI' })).toBeNull()
  })

  it('brings the words of a different asset with it', () => {
    const { rerender } = rig()
    fireEvent.change(field('Title'), { target: { value: 'half typed' } })
    rerender(asset({ id: 'a2', title: 'Blue', altText: 'A blue mug' }))
    expect(field('Title').value).toBe('Blue')
    expect(field('Alt text').value).toBe('A blue mug')
  })

  it('only shows the details without media.manage', () => {
    const { sent } = rig({ canManage: false, value: asset({ focalX: 0.5, focalY: 0.5 }) })
    // disabled through their fieldset, so the elements' own flag stays false
    expect([field('Alt text').matches(':disabled'), field('Title').matches(':disabled')]).toEqual([true, true])
    expect(screen.queryByRole('button', { name: 'Save details' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Reset to centre' })).toBeNull()
    fireEvent.click(screen.getByRole('img', { name: 'A red mug' }), { clientX: 10, clientY: 10 })
    expect(sent).toEqual([])
  })

  it('links to the file and its folder in the library, and copies its link', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal('navigator', { ...navigator, clipboard: { writeText } })
    rig()
    expect(screen.getByRole('link', { name: 'Open' }).getAttribute('href')).toBe('https://cdn/red.png')
    expect(screen.getByRole('link', { name: 'In the library' }).getAttribute('href')).toBe('/media?path=promo')
    fireEvent.click(screen.getByRole('button', { name: 'Copy link' }))
    expect(writeText).toHaveBeenCalledWith('https://cdn/red.png')
    expect(await screen.findByRole('button', { name: 'Copied' })).toBeDefined()
  })

  it('shows a document as a file, with no focal point to set', () => {
    rig({ value: asset({ mimeGroup: 'DOCUMENT', mimeType: 'application/pdf', filename: 'terms.pdf' }) })
    expect(screen.queryByRole('img')).toBeNull()
    expect(screen.getByText('terms.pdf')).toBeDefined()
    expect(screen.getByText('application/pdf')).toBeDefined()
  })
})
