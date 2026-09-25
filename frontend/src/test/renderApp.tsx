import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render } from '@testing-library/react'
import { createMemoryRouter } from 'react-router'
import { RouterProvider } from 'react-router/dom'
import { vi } from 'vitest'
import { routes } from '../router'

/** A canned API answer: a body, or a function of the request URL. */
export type Responder = unknown | ((url: URL) => unknown)

/**
 * Renders the real app (routes, layouts, pages) at `path`, with fetch
 * replaced by a fake API that answers from `api`, keyed by path without the
 * /api/v1 prefix. Unknown paths get the API's 404 problem response. Returns
 * the router (to inspect the URL) and the list of requested URLs.
 */
export function renderApp(path: string, api: Record<string, Responder> = {}) {
  const requests: string[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: string) => {
      const url = new URL(input, 'http://localhost')
      requests.push(url.pathname + url.search)
      const responder = api[url.pathname.replace(/^\/api\/v1/, '')]
      if (responder === undefined) {
        return new Response(JSON.stringify({ title: 'Not Found', status: 404, detail: 'not found' }), {
          status: 404,
          headers: { 'Content-Type': 'application/problem+json' },
        })
      }
      const body = typeof responder === 'function' ? responder(url) : responder
      return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
    }),
  )
  // A fresh cache per test, and no retries, so tests are isolated and fast.
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const router = createMemoryRouter(routes, { initialEntries: [path] })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return { router, requests }
}
