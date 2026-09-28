import { render } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { TabPanelContext } from '@/lib/tab-panel-context'
import {
  confirmCloseTab,
  confirmScopeSwitch,
  hasUnsavedChanges,
  installUnloadGuard,
  mirrorTabTitles,
  unsavedLabels,
  useUnsavedChangesGuard,
  useUnsavedFields,
} from './unsaved-changes'

// What these pin: a dirty page can't be discarded silently by a scope switch, a tab close or the
// browser's own close/refresh - and just as important, that a CLEAN surface never prompts

function Dirty({ isDirty = true, label }: { isDirty?: boolean; label?: string }) {
  useUnsavedChangesGuard(isDirty, label)
  return null
}

function inTab(tabId: string, node: React.ReactNode) {
  return (
    <TabPanelContext.Provider value={{ active: true, visible: true, id: tabId, paneId: 'pane-1' }}>
      {node}
    </TabPanelContext.Provider>
  )
}

afterEach(() => {
  mirrorTabTitles([])
  vi.restoreAllMocks()
})

describe('unsaved-changes registry', () => {
  it('registers while dirty and forgets on unmount', () => {
    const view = render(<Dirty />)
    expect(hasUnsavedChanges()).toBe(true)
    view.unmount()
    expect(hasUnsavedChanges()).toBe(false)
  })

  it('drops the entry as soon as the page goes clean', () => {
    const view = render(<Dirty isDirty />)
    expect(hasUnsavedChanges()).toBe(true)
    view.rerender(<Dirty isDirty={false} />)
    expect(hasUnsavedChanges()).toBe(false)
  })

  it('attributes dirt to the tab whose panel the page sits in', () => {
    const view = render(inTab('tab-a', <Dirty />))
    expect(hasUnsavedChanges('tab-a')).toBe(true)
    expect(hasUnsavedChanges('tab-b')).toBe(false)
    expect(hasUnsavedChanges()).toBe(true) // app-wide still sees it
    view.unmount()
  })

  it('names the page by its tab title when no explicit label was given', () => {
    mirrorTabTitles([{ id: 'tab-a', title: 'Attributes' }])
    const view = render(inTab('tab-a', <Dirty />))
    expect(unsavedLabels()).toEqual(['Attributes'])
    view.unmount()
  })

  it('prefers an explicit label over the tab title', () => {
    mirrorTabTitles([{ id: 'tab-a', title: 'Attributes' }])
    const view = render(inTab('tab-a', <Dirty label="Unsaved campaign changes" />))
    expect(unsavedLabels()).toEqual(['Unsaved campaign changes'])
    view.unmount()
  })

  it('names a page once even when it registers twice', () => {
    mirrorTabTitles([{ id: 'tab-a', title: 'Settings' }])
    // what SettingsPage does - staged-save sections bridge in, and useConfigDrafts guards on top
    const view = render(inTab('tab-a', <><Dirty /><Dirty label="Settings drafts" /></>))
    expect(unsavedLabels()).toEqual(['Settings drafts'])
    view.unmount()
  })

  it('dedupes repeated labels', () => {
    const view = render(
      <>
        <Dirty label="Settings" />
        <Dirty label="Settings" />
      </>,
    )
    expect(unsavedLabels()).toEqual(['Settings'])
    view.unmount()
  })
})

describe('scope switch', () => {
  it('passes straight through when nothing is dirty', () => {
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true)
    expect(confirmScopeSwitch()).toBe(true)
    expect(confirm).not.toHaveBeenCalled()
  })

  it('asks first, and a cancel blocks the switch', () => {
    vi.spyOn(window, 'confirm').mockReturnValue(false)
    const view = render(<Dirty label="Settings" />)
    expect(confirmScopeSwitch()).toBe(false)
    view.unmount()
  })
})

describe('tab close', () => {
  it('only prompts for the tab being closed', () => {
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    const view = render(inTab('tab-a', <Dirty />))
    expect(confirmCloseTab('tab-a')).toBe(false)
    // a sibling tab's edits are none of this prompt's business - it stays mounted and keeps them
    expect(confirmCloseTab('tab-b')).toBe(true)
    expect(confirm).toHaveBeenCalledTimes(1)
    view.unmount()
  })
})

describe('close and refresh', () => {
  function fireBeforeUnload() {
    const event = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(event)
    return event
  }

  it('lets the page go when nothing is dirty', () => {
    const remove = installUnloadGuard()
    expect(fireBeforeUnload().defaultPrevented).toBe(false)
    remove()
  })

  it('cancels the unload while a page is dirty', () => {
    const remove = installUnloadGuard()
    const view = render(<Dirty />)
    expect(fireBeforeUnload().defaultPrevented).toBe(true)
    view.unmount()
    expect(fireBeforeUnload().defaultPrevented).toBe(false)
    remove()
  })
})

// A stored draft already survives a refresh, a close and a scope switch - asking there is noise.
// Closing its workspace tab is the exception: the tab id the draft lives under dies with it
describe('persisted drafts', () => {
  function Persisted() {
    useUnsavedChangesGuard(true, 'Location', { persisted: true })
    return null
  }

  it('lets the browser close and scope switches through', () => {
    const remove = installUnloadGuard()
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    const view = render(inTab('tab-a', <Persisted />))
    const event = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(event)
    expect(event.defaultPrevented).toBe(false)
    expect(confirmScopeSwitch()).toBe(true)
    expect(hasUnsavedChanges()).toBe(false)
    expect(confirm).not.toHaveBeenCalled()
    view.unmount()
    remove()
  })

  it('still prompts when its workspace tab is closed', () => {
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    const view = render(inTab('tab-a', <Persisted />))
    expect(confirmCloseTab('tab-a')).toBe(false)
    expect(confirm).toHaveBeenCalledTimes(1)
    expect(hasUnsavedChanges('tab-a', { includePersisted: true })).toBe(true)
    view.unmount()
  })
})

// Create forms and mount-with-their-record editors have nothing loaded to diff against, so dirt is
// "moved off the value it mounted with" - the baseline must never drift, or a form goes permanently dirty
describe('useUnsavedFields', () => {
  function Form({ value }: { value: string }) {
    useUnsavedFields([value, 'untouched'], 'New thing')
    return null
  }

  it('is clean at its mounted values', () => {
    const view = render(<Form value="" />)
    expect(hasUnsavedChanges()).toBe(false)
    view.unmount()
  })

  it('goes dirty on a change and clean again on the way back', () => {
    const view = render(<Form value="" />)
    view.rerender(<Form value="typed" />)
    expect(hasUnsavedChanges()).toBe(true)
    expect(unsavedLabels()).toEqual(['New thing'])
    // back to what it mounted with - nothing left to lose
    view.rerender(<Form value="" />)
    expect(hasUnsavedChanges()).toBe(false)
    view.unmount()
  })

  it('keeps the ORIGINAL mount values as the baseline, not the latest render', () => {
    const view = render(<Form value="" />)
    view.rerender(<Form value="a" />)
    view.rerender(<Form value="ab" />)
    expect(hasUnsavedChanges()).toBe(true)
    view.unmount()
  })

  it('treats a record already in props as the baseline, so an editor opens clean', () => {
    const view = render(<Form value="loaded name" />)
    expect(hasUnsavedChanges()).toBe(false)
    view.unmount()
  })
})
