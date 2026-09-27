import { beforeEach, describe, expect, it } from 'vitest'
import { Observable, firstValueFrom } from 'rxjs'
import { ApolloLink } from '@apollo/client'
import { parse } from 'graphql'
import { setSignalsDown, signalsIsDown } from './health'
import { signalsHealthLink, SIGNALS_RELAYED_EXTENSION } from './health-link'

function answer(result: ApolloLink.Result) {
  return new ApolloLink(() => new Observable((o) => { o.next(result); o.complete() }))
}

function run(result: ApolloLink.Result) {
  const link = signalsHealthLink.concat(answer(result))
  return firstValueFrom(ApolloLink.execute(link, { query: parse('query Q { x }') }, { client: {} as never }))
}

describe('signalsHealthLink', () => {
  beforeEach(() => setSignalsDown(true))

  it('leaves the banner up for a core-only answer', async () => {
    await run({ data: { x: 1 } })
    expect(signalsIsDown()).toBe(true)
  })

  it('clears the banner when a relayed operation answers', async () => {
    await run({ data: { x: 1 }, extensions: { [SIGNALS_RELAYED_EXTENSION]: true } })
    expect(signalsIsDown()).toBe(false)
  })

  it('keeps the banner when a relayed operation still reports the suite unreachable', async () => {
    await run({
      data: null,
      errors: [{ message: 'down', extensions: { code: 'SIGNALS_UNAVAILABLE' } }],
      extensions: { [SIGNALS_RELAYED_EXTENSION]: true },
    })
    expect(signalsIsDown()).toBe(true)
  })

  it('passes the result through untouched', async () => {
    const result = { data: { x: 2 } }
    expect(await run(result)).toEqual(result)
  })
})
