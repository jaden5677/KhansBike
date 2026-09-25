import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render } from '@testing-library/react'
import { StrictMode } from 'react'
import { createMemoryRouter } from 'react-router'
import { RouterProvider } from 'react-router/dom'
import { vi } from 'vitest'
import { routes } from '../router'

/** A non-200 answer; plain values are sent as 200 JSON. */
export class Reply {
  constructor(
    readonly status: number,
    readonly body?: unknown,
  ) {}
}

/** An API error in the server's problem+json shape. */
export function problem(status: number, detail: string, errors?: { field: string; message: string }[]) {
  return new Reply(status, { title: detail, status, detail, errors })
}

/** A canned API answer: a body or Reply, or a function of the request. */
export type Responder = unknown | ((url: URL, call: Call) => unknown)

export interface Call {
  method: string
  path: string
  body?: unknown
  headers: Headers
}

/**
 * Renders the real app (routes, layouts, pages) at `path`, with fetch
 * replaced by a fake API that answers from `api`, keyed by path without the
 * /api/v1 prefix. Unknown paths get the API's 404 problem response. Returns
 * the router (to inspect the URL), the requested URLs, and every call with
 * its method, body and headers.
 */
export function renderApp(path: string, api: Record<string, Responder> = {}) {
  const requests: string[] = []
  const calls: Call[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: string, init: RequestInit = {}) => {
      const url = new URL(input, 'http://localhost')
      const call: Call = {
        method: init.method ?? 'GET',
        path: url.pathname.replace(/^\/api\/v1/, ''),
        body: typeof init.body === 'string' ? JSON.parse(init.body) : undefined,
        headers: new Headers(init.headers),
      }
      requests.push(url.pathname + url.search)
      calls.push(call)
      const responder = api[call.path]
      let answer = typeof responder === 'function' ? responder(url, call) : responder
      if (answer === undefined) answer = problem(404, 'not found')
      const { status, body } = answer instanceof Reply ? answer : new Reply(200, answer)
      if (body === undefined) return new Response(null, { status })
      const type = status >= 400 ? 'application/problem+json' : 'application/json'
      return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': type } })
    }),
  )
  // A fresh cache per test, and no retries, so tests are isolated and fast.
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const router = createMemoryRouter(routes, { initialEntries: [path] })
  // StrictMode as in main.tsx, so effects run twice here just as in development.
  render(
    <StrictMode>
      <QueryClientProvider client={queryClient}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    </StrictMode>,
  )
  return { router, requests, calls }
}
