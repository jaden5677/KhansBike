import type { ReactNode } from 'react'
import { ApiError } from '../api/client'
import { NotFound } from '../pages/NotFound'

/** The parts of a TanStack Query result this component needs. */
interface QueryLike<T> {
  data: T | undefined
  error: Error | null
  isPending: boolean
  refetch: () => unknown
}

/**
 * Renders the loading and error states of a query the same way everywhere,
 * and hands the data to `children` once it has arrived (a "render prop").
 * A 404 from the API shows the not-found page, so a mistyped product link
 * looks the same as a mistyped page address.
 */
export function Loadable<T>({ query, children }: { query: QueryLike<T>; children: (data: T) => ReactNode }) {
  if (query.error) {
    if (query.error instanceof ApiError && query.error.status === 404) return <NotFound />
    return (
      <div role="alert" className="notice">
        <p>{query.error instanceof ApiError ? query.error.message : 'Something went wrong.'}</p>
        <button type="button" onClick={() => query.refetch()}>
          Try again
        </button>
      </div>
    )
  }
  if (query.isPending || query.data === undefined) {
    return (
      <p role="status" className="muted">
        Loading…
      </p>
    )
  }
  return <>{children(query.data)}</>
}
