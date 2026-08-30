import { useEffect } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { Link, Outlet, RouterProvider, createMemoryRouter } from 'react-router-dom'
import { allowNextNavigation, duringCommit, useNavGuard } from './nav-guard'
import { useUnsavedChangesGuard } from './unsaved-changes'

// The point of blocking rather than warning-after-the-fact: a refused navigation must leave the
// dirty page MOUNTED, drafts and all - these assert the page instance survives a cancelled leave

function Root() {
  useNavGuard()
  return <Outlet />
}

let mounts = 0

function Editor({ dirty = true }: { dirty?: boolean }) {
  // A remount is exactly what a blocked navigation must NOT cause, so it's the thing to count
  useEffect(() => {
    mounts++
  }, [])
  useUnsavedChangesGuard(dirty)
  return (
    <div>
      <span>editor</span>
      <Link to="/list">to list</Link>
      <Link to="/edit?page=2">next page</Link>
    </div>
  )
}

function renderAt(dirty: boolean, entries = ['/edit'], index?: number) {
  mounts = 0
  const router = createMemoryRouter(
    [
      {
        element: <Root />,
        children: [
          { path: '/edit', element: <Editor dirty={dirty} /> },
          { path: '/list', element: <span>list</span> },
        ],
      },
    ],
    { initialEntries: entries, initialIndex: index },
  )
  return { router, ...render(<RouterProvider router={router} />) }
}

afterEach(() => vi.restoreAllMocks())

describe('in-app navigation guard', () => {
  it('lets a clean page navigate away untouched', async () => {
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true)
    renderAt(false)
    fireEvent.click(screen.getByText('to list'))
    expect(await screen.findByText('list')).toBeTruthy()
    expect(confirm).not.toHaveBeenCalled()
  })

  it('blocks a dirty page, and a cancel keeps the very same page mounted', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(false)
    const { router } = renderAt(true)
    expect(mounts).toBe(1)

    fireEvent.click(screen.getByText('to list'))
    await waitFor(() => expect(window.confirm).toHaveBeenCalled())

    expect(screen.getByText('editor')).toBeTruthy()
    expect(router.state.location.pathname).toBe('/edit')
    expect(mounts).toBe(1) // never remounted, so its drafts are still there
  })

  it('goes through once the discard is confirmed', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    renderAt(true)
    fireEvent.click(screen.getByText('to list'))
    expect(await screen.findByText('list')).toBeTruthy()
  })

  it('ignores a search-only change - filters and paging keep the page mounted', async () => {
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    const { router } = renderAt(true)
    fireEvent.click(screen.getByText('next page'))
    await waitFor(() => expect(router.state.location.search).toBe('?page=2'))
    expect(confirm).not.toHaveBeenCalled()
    expect(mounts).toBe(1)
  })

  it('waves through a navigation the workspace armed - switching tabs unmounts nothing', async () => {
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    const { router } = renderAt(true)
    allowNextNavigation()
    router.navigate('/list')
    expect(await screen.findByText('list')).toBeTruthy()
    expect(confirm).not.toHaveBeenCalled()
  })

  it('waves through a page leaving from inside its own save', async () => {
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    const { router } = renderAt(true)
    await duringCommit(async () => {
      await router.navigate('/list')
    })
    expect(await screen.findByText('list')).toBeTruthy()
    expect(confirm).not.toHaveBeenCalled()
  })

  // The reason this is a data router at all - react-router rewinds history itself and never
  // commits the POP, where a popstate listener could only navigate BACK after the page was gone
  it('blocks the back button without unmounting the page', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(false)
    const { router } = renderAt(true, ['/list', '/edit'], 1)
    expect(mounts).toBe(1)

    await router.navigate(-1)
    await waitFor(() => expect(window.confirm).toHaveBeenCalled())

    expect(screen.getByText('editor')).toBeTruthy()
    expect(router.state.location.pathname).toBe('/edit')
    expect(mounts).toBe(1)
  })
})
