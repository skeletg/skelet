import { Observable } from 'rxjs'
import { ApolloLink } from '@apollo/client'
import { setSignalsDown } from './health'

// Core stamps this extension on any response whose resolvers reached for the suite
export const SIGNALS_RELAYED_EXTENSION = 'signalsRelayed'

// Clears the banner only when an operation relayed to Signals came back without SIGNALS_UNAVAILABLE
export const signalsHealthLink = new ApolloLink(
  (operation, forward) =>
    new Observable<ApolloLink.Result>((observer) => {
      const sub = forward(operation).subscribe({
        next: (result) => {
          const errs = 'errors' in result ? (result.errors ?? []) : []
          const relayed = result.extensions?.[SIGNALS_RELAYED_EXTENSION] === true
          if (relayed && !errs.some((e) => e.extensions?.code === 'SIGNALS_UNAVAILABLE')) setSignalsDown(false)
          observer.next(result)
        },
        error: (err) => observer.error(err),
        complete: () => observer.complete(),
      })
      return () => sub.unsubscribe()
    }),
)
