import { describe, expect, it } from 'vitest'
import { effectiveMediaRoles } from './media-utils'

const none = { MAIN: null, THUMBNAIL: null, OG_IMAGE: null }

describe('effectiveMediaRoles', () => {
  it('falls every role back to the only ready image', () => {
    expect(effectiveMediaRoles([{ id: 'a', status: 'READY' }], none)).toEqual({
      MAIN: { id: 'a', auto: true },
      THUMBNAIL: { id: 'a', auto: true },
      OG_IMAGE: { id: 'a', auto: true },
    })
  })

  it('keeps explicit roles, thumbnail follows main, OG follows the first image', () => {
    const media = [{ id: 'a', status: 'READY' }, { id: 'b', status: 'READY' }]
    expect(effectiveMediaRoles(media, { ...none, MAIN: 'b' })).toEqual({
      MAIN: { id: 'b', auto: false },
      THUMBNAIL: { id: 'b', auto: true },
      OG_IMAGE: { id: 'a', auto: true },
    })
  })

  it('skips pending images and reports nothing when none are ready', () => {
    expect(effectiveMediaRoles([{ id: 'p', status: 'PENDING' }], none)).toEqual({
      MAIN: { id: null, auto: false },
      THUMBNAIL: { id: null, auto: false },
      OG_IMAGE: { id: null, auto: false },
    })
  })
})
