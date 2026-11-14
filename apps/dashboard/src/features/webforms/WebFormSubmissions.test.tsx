import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { STORAGE_KEY } from '@/shared/auth/auth'
import { downloadServer } from '@/test/downloads'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { WebFormDocument, WebFormSubmissionsDocument } from './WebForms.gql'
import { WebFormSubmissions } from './WebFormSubmissions'

const form = {
  __typename: 'WebForm', id: 'f1', channelId: 'ch1', slug: 'apply', name: 'Job application', status: 'ACTIVE', submissionCount: 2,
  fields: [{ key: 'name', type: 'text', label: 'Name' }, { key: 'cv', type: 'file', label: 'CV' }],
  settings: {}, createdAt: '2026-10-01T00:00:00Z', updatedAt: '2026-10-01T00:00:00Z', deletedAt: null,
}
const submission = (id: string, name: string, files: unknown) => ({
  __typename: 'WebFormSubmission', id, formId: 'f1', answers: { name }, files, customerId: null, meta: {}, createdAt: '2026-11-01T10:00:00Z',
})

function page(submissions: object[]) {
  return renderPage({
    path: '/cms/forms/f1/submissions',
    adminId: 'admin-1',
    routes: { '/cms/forms/:id/submissions': <WebFormSubmissions /> },
    mocks: [
      meMock(),
      { request: { query: WebFormDocument, variables: { id: 'f1' } }, result: { data: { webForm: form } }, maxUsageCount: 5 },
      { request: { query: WebFormSubmissionsDocument, variables: { formId: 'f1', limit: 200, offset: 0 } }, result: { data: { webFormSubmissions: submissions } }, maxUsageCount: 5 },
    ],
  })
}

beforeEach(() => {
  localStorage.setItem(STORAGE_KEY, JSON.stringify({ accessToken: 'tok', refreshToken: 'r', admin: {} }))
  URL.createObjectURL = vi.fn(() => 'blob:file')
  URL.revokeObjectURL = vi.fn()
})
afterEach(() => {
  localStorage.clear()
  vi.unstubAllGlobals()
})

describe('WebFormSubmissions files', () => {
  it('downloads a submitted file through the API, with the session', async () => {
    const { asked, saved } = downloadServer(() => new Response('%PDF', { headers: { 'content-disposition': 'attachment; filename="Ana CV.pdf"' } }))
    page([
      submission('s1', 'Ana', [{ fieldKey: 'cv', objectKey: 'webforms/f1/cv/u1-cv.pdf', name: 'cv.pdf', size: 20480, mimeType: 'application/pdf' }]),
      submission('s2', 'Bo', []),
    ])

    const row = (await screen.findByText('Ana', {}, SLOW)).closest('tr')!
    fireEvent.click(within(row).getByRole('button', { name: /cv\.pdf/ }))
    const file = '/api/admin/webforms/submissions/s1/files/cv'
    await waitFor(() => expect(saved).toEqual([{ href: `${file}?ticket=t1`, name: 'Ana CV.pdf' }]))
    expect(asked).toEqual([
      { url: '/api/admin/download-ticket', method: 'POST', authorization: 'Bearer tok' },
      { url: `http://localhost:3000${file}?ticket=t1`, method: 'GET', authorization: undefined },
    ])
    expect(within(row).getByRole('button', { name: /cv\.pdf/ }).textContent).toContain('20.0 KB')
    expect(within(screen.getByText('Bo').closest('tr')!).queryByRole('button')).toBeNull()
    expect(screen.queryByRole('columnheader', { name: /CV/ })).toBeNull()
    expect(screen.getByRole('columnheader', { name: /Name/ })).toBeDefined()
  })

  it('says why when a file cannot be had', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response("that file isn't there", { status: 404 })))
    page([submission('s1', 'Ana', [{ fieldKey: 'cv', name: 'cv.pdf', size: 1 }])])
    fireEvent.click(await screen.findByRole('button', { name: /cv\.pdf/ }, SLOW))
    expect(await screen.findByText("cv.pdf: that file isn't there", {}, SLOW)).toBeDefined()
  })

  it('shows no files column for a form nobody sent a file to, and survives misshapen file data', async () => {
    page([submission('s1', 'Ana', null), submission('s2', 'Bo', [{ name: 'no field key' }, 'junk'])])
    await screen.findByText('Ana', {}, SLOW)
    expect(screen.queryByRole('columnheader', { name: 'Files' })).toBeNull()
  })
})
