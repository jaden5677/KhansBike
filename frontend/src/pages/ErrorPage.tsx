import { Link, useRouteError } from 'react-router'
import { ApiError } from '../api/client'

/**
 * Shown when a page throws while rendering or loading. API errors carry a
 * message written for people; anything else is a bug, so we show a generic
 * message and leave the details to the browser console.
 */
export function ErrorPage() {
  const error = useRouteError()
  const message = error instanceof ApiError ? error.message : 'Something went wrong on our side.'
  if (!(error instanceof ApiError)) console.error(error)
  return (
    <section className="content">
      <h1>Sorry</h1>
      <p>{message}</p>
      <p>
        <Link to="/">Back to the shop</Link>
      </p>
    </section>
  )
}
