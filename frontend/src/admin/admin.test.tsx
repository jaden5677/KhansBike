import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { AdminProduct, FormSchema } from '../api/adminTypes'
import type { Session } from '../api/types'
import { problem, Reply, renderApp, type Responder } from '../test/renderApp'
import { safeNext } from './LoginPage'
import { describeField } from './ProductEditorPage'

const session: Session = {
  user: { id: 'u1', email: 'owner@example.com', displayName: 'Owner', role: 'admin' },
  kind: 'admin',
  csrfToken: 'csrf-1',
}

const categories = {
  items: [
    { id: 'c-grips', parentId: null, name: 'Grips', slug: 'grips', path: 'grips', position: 0, isActive: true },
  ],
}

const schema: FormSchema = {
  categoryId: 'c-grips',
  version: 1,
  fields: [
    { key: 'material', label: 'Material', inputType: 'text', dataType: 'text', required: false, isVariantAxis: false },
    {
      key: 'colour',
      label: 'Colour',
      inputType: 'select',
      dataType: 'color',
      required: true,
      isVariantAxis: true,
      options: [
        { value: 'black', label: 'Black' },
        { value: 'red', label: 'Red' },
      ],
    },
  ],
}

const product: AdminProduct = {
  id: 'p1',
  categoryId: 'c-grips',
  category: { id: 'c-grips', name: 'Grips', slug: 'grips' },
  brandId: null,
  name: 'Star Grips',
  slug: 'star-grips',
  summary: null,
  description: null,
  status: 'active',
  isFeatured: false,
  retailPriceIsPublic: true,
  attributes: {},
  variants: [
    {
      id: 'v1',
      sku: 'SG-BK',
      supplierId: null,
      supplierItemNo: null,
      modelNo: null,
      nameSuffix: null,
      position: 0,
      stockStatus: 'in_stock',
      isDefault: true,
      attributes: { colour: 'black' },
      prices: { retail_ttd: { amount: '45.00', currency: 'TTD', effectiveFrom: '2026-09-01' } },
    },
  ],
  media: [],
  createdAt: '2026-09-01T00:00:00Z',
  updatedAt: '2026-09-02T00:00:00Z',
  publishedAt: null,
}

/** The endpoints every signed-in admin screen here touches. */
function adminAPI(extra: Record<string, Responder> = {}): Record<string, Responder> {
  return {
    '/auth/session': session,
    '/admin/categories': categories,
    '/admin/categories/c-grips/form-schema': schema,
    '/admin/brands': { items: [] },
    '/admin/suppliers': { items: [] },
    '/admin/products': { items: [], total: 0 },
    '/admin/subscribers/stats': { confirmed: 3 },
    ...extra,
  }
}

describe('sign-in', () => {
  it('signs in and returns to the page that asked for it', async () => {
    let signedIn = false
    const { router } = renderApp('/admin/login?next=%2Fadmin%2Fproducts', {
      ...adminAPI(),
      '/auth/session': () => (signedIn ? session : problem(401, 'sign in')),
      '/auth/login': () => {
        signedIn = true
        return session
      },
    })
    fireEvent.change(await screen.findByLabelText('Email'), { target: { value: 'owner@example.com' } })
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'correct horse' } })
    fireEvent.click(screen.getByRole('button', { name: 'Sign in' }))
    await waitFor(() => expect(router.state.location.pathname).toBe('/admin/products'))
    expect(await screen.findByRole('heading', { name: 'Products' })).toBeInTheDocument()
  })

  it('says the same thing for a wrong email or password', async () => {
    renderApp('/admin/login', {
      '/auth/session': problem(401, 'sign in'),
      '/auth/login': problem(401, 'invalid credentials'),
    })
    fireEvent.change(await screen.findByLabelText('Email'), { target: { value: 'x@example.com' } })
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'nope' } })
    fireEvent.click(screen.getByRole('button', { name: 'Sign in' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Wrong email or password.')
  })

  it('only returns to pages inside the admin (no open redirect)', () => {
    expect(safeNext('/admin/products?status=draft')).toBe('/admin/products?status=draft')
    expect(safeNext('https://evil.example')).toBe('/admin')
    expect(safeNext('//evil.example/admin')).toBe('/admin')
    expect(safeNext(null)).toBe('/admin')
  })

  it('goes back to sign-in when the session expires mid-visit', async () => {
    let expired = false
    const { router } = renderApp('/admin', {
      ...adminAPI(),
      '/auth/session': () => (expired ? problem(401, 'sign in') : session),
      '/admin/products': () => {
        expired = true
        return problem(401, 'session expired')
      },
    })
    await waitFor(() => expect(router.state.location.pathname).toBe('/admin/login'))
  })

  it('offers "Unpair" instead of "Sign out" on a paired phone', async () => {
    renderApp('/admin', adminAPI({ '/auth/session': { ...session, kind: 'device', csrfToken: undefined } }))
    expect(await screen.findByRole('button', { name: 'Unpair' })).toBeInTheDocument()
  })
})

describe('product editor', () => {
  it('creates a product with a variant from the category form', async () => {
    const { calls, router } = renderApp(
      '/admin/products/new',
      adminAPI({
        '/admin/products': (_url: URL, call: { method: string }) =>
          call.method === 'POST' ? new Reply(201, product, { ETag: '"v1"' }) : { items: [], total: 0 },
        '/admin/products/p1': new Reply(200, product, { ETag: '"v1"' }),
      }),
    )
    fireEvent.change(await screen.findByLabelText('Name'), { target: { value: 'Star Grips' } })
    await screen.findByRole('option', { name: 'Grips' })
    fireEvent.change(screen.getByLabelText('Category'), { target: { value: 'c-grips' } })
    // The category's form: product-level Material, per-variant Colour.
    fireEvent.change(await screen.findByLabelText('Material'), { target: { value: 'rubber' } })
    const variant = screen.getByRole('group', { name: 'Variant 1' })
    fireEvent.change(within(variant).getByLabelText(/Colour/), { target: { value: 'black' } })
    fireEvent.change(within(variant).getByLabelText('SKU'), { target: { value: 'SG-BK' } })
    fireEvent.change(within(variant).getByLabelText('Retail (TTD)'), { target: { value: '45' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create product' }))

    await waitFor(() => expect(router.state.location.pathname).toBe('/admin/products/p1'))
    const post = calls.find((c) => c.path === '/admin/products' && c.method === 'POST')!
    expect(post.headers.get('X-CSRF-Token')).toBe('csrf-1')
    expect(post.body).toMatchObject({
      name: 'Star Grips',
      categoryId: 'c-grips',
      status: 'draft',
      attributes: { material: 'rubber' },
      variants: [{ sku: 'SG-BK', isDefault: true, attributes: { colour: 'black' }, prices: { retail_ttd: '45' } }],
    })
    expect(await screen.findByText('Product created.')).toBeInTheDocument()
  })

  it("sends the ETag back and, if someone else saved first, offers their version", async () => {
    let version = 1
    const { calls } = renderApp(
      '/admin/products/p1',
      adminAPI({
        '/admin/products/p1': (_url: URL, call: { method: string }) =>
          call.method === 'PUT'
            ? problem(412, 'the product was changed by someone else')
            : new Reply(200, { ...product, name: version === 1 ? 'Star Grips' : 'Star Grips Pro' }, { ETag: `"v${version}"` }),
      }),
    )
    fireEvent.change(await screen.findByDisplayValue('Star Grips'), { target: { value: 'My name' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))

    expect(await screen.findByText(/Someone else saved this product/)).toBeInTheDocument()
    expect(calls.find((c) => c.method === 'PUT')?.headers.get('If-Match')).toBe('"v1"')

    version = 2
    fireEvent.click(screen.getByRole('button', { name: 'Reload their version' }))
    expect(await screen.findByDisplayValue('Star Grips Pro')).toBeInTheDocument()
    expect(screen.queryByText(/Someone else saved/)).not.toBeInTheDocument()
  })

  it("lists the server's problems and shows each beside its field", async () => {
    renderApp(
      '/admin/products/p1',
      adminAPI({
        '/admin/products/p1': (_url: URL, call: { method: string }) =>
          call.method === 'PUT'
            ? problem(422, 'Validation failed', [{ field: 'variants[0].sku', message: 'duplicates another variant' }])
            : new Reply(200, product, { ETag: '"v1"' }),
      }),
    )
    fireEvent.change(await screen.findByDisplayValue('SG-BK'), { target: { value: 'SG-RD' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    expect(await screen.findByText('Variant 1 › SKU duplicates another variant')).toBeInTheDocument()
    const variant = screen.getByRole('group', { name: 'Variant 1' })
    expect(within(variant).getByText('duplicates another variant')).toBeInTheDocument()
  })

  it('asks before leaving with unsaved changes', async () => {
    const { router } = renderApp('/admin/products/p1', adminAPI({ '/admin/products/p1': new Reply(200, product, { ETag: '"v1"' }) }))
    fireEvent.change(await screen.findByDisplayValue('Star Grips'), { target: { value: 'Changed' } })
    fireEvent.click(screen.getByRole('link', { name: '← Products' }))
    const dialog = await screen.findByRole('alertdialog', { name: 'Unsaved changes' })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Stay' }))
    expect(router.state.location.pathname).toBe('/admin/products/p1')

    fireEvent.click(screen.getByRole('link', { name: '← Products' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Leave without saving' }))
    await waitFor(() => expect(router.state.location.pathname).toBe('/admin/products'))
  })
})

describe('describeField', () => {
  it('turns API field paths into readable names', () => {
    expect(describeField('variants[1].prices.retail_ttd')).toBe('Variant 2 › Retail (TTD)')
    expect(describeField('attributes.colour', schema)).toBe('Colour')
    expect(describeField('name')).toBe('Name')
  })
})
