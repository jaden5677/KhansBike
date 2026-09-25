// The one place the app talks to the Go API. Pages and hooks call api.get /
// api.post / ... and never fetch() directly, so the rules below live here only:
//
//   - JSON in and out; FormData bodies (uploads) are passed through as-is.
//   - Credentials: a browser session sends its cookie automatically and must
//     add X-CSRF-Token on unsafe methods; a paired phone sends a Bearer token.
//   - Errors: the API answers failures with application/problem+json
//     (RFC 9457); they become an ApiError carrying any per-field messages.
//   - Concurrency: responses expose their ETag so an editor can send it back
//     as If-Match and get a 412 instead of overwriting someone else's save.

export const API_BASE = '/api/v1'

export interface FieldError {
  field: string
  message: string
}

/** A failed API call. status is 0 when the server could not be reached. */
export class ApiError extends Error {
  readonly status: number
  readonly title: string
  readonly detail?: string
  readonly fieldErrors: FieldError[]

  constructor(status: number, title: string, detail?: string, fieldErrors: FieldError[] = []) {
    super(detail ?? title)
    this.name = 'ApiError'
    this.status = status
    this.title = title
    this.detail = detail
    this.fieldErrors = fieldErrors
  }

  /** The message for one form field, if the server rejected it. */
  fieldMessage(field: string): string | undefined {
    return this.fieldErrors.find((e) => e.field === field)?.message
  }
}

// Credentials are held in memory. The auth layer sets them after login,
// session restore or pairing; the CSRF token is never persisted.
let csrfToken: string | undefined
let deviceToken: string | undefined

export function setCsrfToken(token: string | undefined): void {
  csrfToken = token
}

export function setDeviceToken(token: string | undefined): void {
  deviceToken = token
}

export type QueryValue = string | number | boolean | undefined | null | readonly string[]
export type Query = URLSearchParams | Record<string, QueryValue>

export interface RequestOptions {
  query?: Query
  body?: unknown
  /** The ETag from the GET, to reject the write if the record changed since. */
  ifMatch?: string
  signal?: AbortSignal
}

export interface ApiResponse<T> {
  data: T
  etag?: string
}

type Method = 'GET' | 'POST' | 'PUT' | 'DELETE'

/** Builds /api/v1<path>?query, dropping empty values and repeating arrays. */
export function apiURL(path: string, query?: Query): string {
  const params = new URLSearchParams()
  if (query instanceof URLSearchParams) {
    query.forEach((value, key) => params.append(key, value))
  } else if (query) {
    for (const [key, value] of Object.entries(query)) {
      if (value === undefined || value === null || value === '') continue
      if (Array.isArray(value)) value.forEach((v) => params.append(key, v))
      else params.append(key, String(value))
    }
  }
  const qs = params.toString()
  return API_BASE + path + (qs ? `?${qs}` : '')
}

export async function request<T>(method: Method, path: string, opts: RequestOptions = {}): Promise<ApiResponse<T>> {
  const headers = new Headers({ Accept: 'application/json, application/problem+json' })
  let body: BodyInit | undefined
  if (opts.body instanceof FormData) {
    body = opts.body // the browser sets the multipart boundary itself
  } else if (opts.body !== undefined) {
    headers.set('Content-Type', 'application/json')
    body = JSON.stringify(opts.body)
  }
  if (deviceToken) {
    headers.set('Authorization', `Bearer ${deviceToken}`)
  } else if (csrfToken && method !== 'GET') {
    headers.set('X-CSRF-Token', csrfToken)
  }
  if (opts.ifMatch) headers.set('If-Match', opts.ifMatch)

  let res: Response
  try {
    res = await fetch(apiURL(path, opts.query), {
      method,
      headers,
      body,
      signal: opts.signal,
      credentials: 'same-origin',
    })
  } catch (err) {
    if (err instanceof DOMException && err.name === 'AbortError') throw err
    throw new ApiError(0, 'Network error', 'Could not reach the server. Check your connection and try again.')
  }

  if (!res.ok) throw await toApiError(res)
  const etag = res.headers.get('ETag') ?? undefined
  const isJSON = res.headers.get('Content-Type')?.includes('json') ?? false
  if (res.status === 204 || !isJSON) return { data: undefined as T, etag }
  return { data: (await res.json()) as T, etag }
}

async function toApiError(res: Response): Promise<ApiError> {
  if (res.headers.get('Content-Type')?.includes('json')) {
    try {
      const p = (await res.json()) as { title?: string; detail?: string; errors?: FieldError[] }
      return new ApiError(res.status, p.title ?? res.statusText, p.detail, p.errors ?? [])
    } catch {
      // Fall through: a malformed body still has a status worth reporting.
    }
  }
  return new ApiError(res.status, res.statusText || 'Request failed')
}

/** Shorthands returning just the body; use request() when you need the ETag. */
export const api = {
  get: <T>(path: string, query?: Query, signal?: AbortSignal) =>
    request<T>('GET', path, { query, signal }).then((r) => r.data),
  post: <T>(path: string, body?: unknown) => request<T>('POST', path, { body }).then((r) => r.data),
  put: <T>(path: string, body: unknown, ifMatch?: string) =>
    request<T>('PUT', path, { body, ifMatch }).then((r) => r.data),
  delete: (path: string) => request<void>('DELETE', path).then(() => undefined),
}
