import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError, apiURL, request, setCsrfToken, setDeviceToken } from './client'

function jsonResponse(status: number, body: unknown, headers: Record<string, string> = {}) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json', ...headers },
  })
}

/** Replaces fetch with a stub and returns it so tests can inspect the call. */
function stubFetch(response: Response | Error) {
  const fetchMock = vi.fn(async (_url: string, _init?: RequestInit) => {
    if (response instanceof Error) throw response
    return response
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function sentHeaders(fetchMock: ReturnType<typeof stubFetch>) {
  return fetchMock.mock.calls[0][1]!.headers as Headers
}

afterEach(() => {
  vi.unstubAllGlobals()
  setCsrfToken(undefined)
  setDeviceToken(undefined)
})

describe('apiURL', () => {
  it('drops empty values and repeats arrays', () => {
    expect(apiURL('/products', { category: 'tyres', q: '', brand: undefined, 'attr.colour': ['red', 'blue'] })).toBe(
      '/api/v1/products?category=tyres&attr.colour=red&attr.colour=blue',
    )
  })

  it('forwards URLSearchParams unchanged', () => {
    expect(apiURL('/products', new URLSearchParams('attr.size_min=26&sort=name'))).toBe(
      '/api/v1/products?attr.size_min=26&sort=name',
    )
  })
})

describe('request', () => {
  it('returns the body and ETag', async () => {
    stubFetch(jsonResponse(200, { id: 'p1' }, { ETag: '"abc"' }))
    await expect(request('GET', '/admin/products/p1')).resolves.toEqual({ data: { id: 'p1' }, etag: '"abc"' })
  })

  it('sends JSON, the CSRF token on writes, and If-Match', async () => {
    setCsrfToken('csrf-1')
    const fetchMock = stubFetch(jsonResponse(200, {}))
    await api.put('/admin/products/p1', { name: 'Rider' }, '"abc"')
    const init = fetchMock.mock.calls[0][1]!
    const headers = sentHeaders(fetchMock)
    expect(init.method).toBe('PUT')
    expect(init.body).toBe('{"name":"Rider"}')
    expect(headers.get('Content-Type')).toBe('application/json')
    expect(headers.get('X-CSRF-Token')).toBe('csrf-1')
    expect(headers.get('If-Match')).toBe('"abc"')
  })

  it('leaves the CSRF token off reads', async () => {
    setCsrfToken('csrf-1')
    const fetchMock = stubFetch(jsonResponse(200, {}))
    await api.get('/admin/products')
    expect(sentHeaders(fetchMock).has('X-CSRF-Token')).toBe(false)
  })

  it('uses the Bearer token for a paired phone', async () => {
    setDeviceToken('dev-1')
    const fetchMock = stubFetch(jsonResponse(200, {}))
    await api.post('/admin/media')
    expect(sentHeaders(fetchMock).get('Authorization')).toBe('Bearer dev-1')
  })

  it('passes FormData through without a JSON content type', async () => {
    const fetchMock = stubFetch(jsonResponse(201, {}))
    const form = new FormData()
    form.append('file', new Blob(['x']), 'a.jpg')
    await api.post('/admin/media', form)
    expect(fetchMock.mock.calls[0][1]!.body).toBe(form)
    expect(sentHeaders(fetchMock).has('Content-Type')).toBe(false)
  })

  it('resolves 204 responses to undefined', async () => {
    stubFetch(new Response(null, { status: 204 }))
    await expect(api.delete('/admin/brands/b1')).resolves.toBeUndefined()
  })

  it('turns problem+json into an ApiError with field errors', async () => {
    stubFetch(
      jsonResponse(
        422,
        { title: 'Validation failed', status: 422, errors: [{ field: 'name', message: 'is required' }] },
        { 'Content-Type': 'application/problem+json' },
      ),
    )
    const err = await api.post('/admin/brands', {}).catch((e: unknown) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect((err as ApiError).status).toBe(422)
    expect((err as ApiError).fieldMessage('name')).toBe('is required')
  })

  it('reports an unreachable server as status 0', async () => {
    stubFetch(new TypeError('Failed to fetch'))
    await expect(api.get('/categories')).rejects.toMatchObject({ status: 0, title: 'Network error' })
  })
})
