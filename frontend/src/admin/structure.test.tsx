import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { AdminAttribute, AdminCategory, AdminProduct, Binding } from '../api/adminTypes'
import type { Session } from '../api/types'
import { Reply, renderApp, type Call, type Responder } from '../test/renderApp'
import { optionValue } from './AttributeEditPage'
import { keyFromLabel } from './AttributesPage'

const session: Session = {
  user: { id: 'u1', email: 'owner@example.com', displayName: 'Owner', role: 'admin' },
  kind: 'admin',
  csrfToken: 'csrf-1',
}

function category(id: string, name: string, path: string, parentId: string | null = null): AdminCategory {
  return { id, name, slug: id, path, parentId, position: 0, description: null, heroAssetId: null, isActive: true, updatedAt: '' }
}

const wheels = category('wheels', 'Wheels', 'wheels')
const tyres = category('tyres', 'Tyres', 'wheels.tyres', 'wheels')
const tubes = category('tubes', 'Tubes', 'wheels.tyres.tubes', 'tyres')
const grips = category('grips', 'Grips', 'grips')

const colour: AdminAttribute = {
  id: 'a-colour',
  key: 'colour',
  label: 'Colour',
  dataType: 'color',
  unit: null,
  inputType: 'swatch',
  isFilterable: true,
  isSearchable: false,
  helpText: null,
  options: [],
  updatedAt: '',
}

const binding: Binding = {
  attribute: colour,
  position: 0,
  isRequired: false,
  isVariantAxis: false,
  labelOverride: null,
  effectiveLabel: 'Colour',
}

function api(extra: Record<string, Responder> = {}): Record<string, Responder> {
  return {
    '/auth/session': session,
    '/admin/categories': { items: [wheels, tyres, tubes, grips] },
    '/admin/categories/tyres': new Reply(200, tyres, { ETag: '"c1"' }),
    '/admin/categories/tyres/attributes': { items: [binding] },
    '/admin/attributes': { items: [colour] },
    '/admin/brands': { items: [] },
    '/admin/suppliers': { items: [] },
    ...extra,
  }
}

const find = (calls: Call[], method: string, path: string) => calls.find((c) => c.method === method && c.path === path)

describe('categories', () => {
  it('moves a category, offering only parents outside its own subtree', async () => {
    const { calls } = renderApp('/admin/categories/tyres', {
      ...api(),
      '/admin/categories/tyres': (_u: URL, c: Call) =>
        c.method === 'PUT' ? new Reply(200, { ...tyres, parentId: null }, { ETag: '"c2"' }) : new Reply(200, tyres, { ETag: '"c1"' }),
    })
    const parent = await screen.findByRole('combobox', { name: /Inside/ })
    await within(parent).findByRole('option', { name: /Grips/ })
    const names = within(parent).getAllByRole('option').map((o) => o.textContent?.trim())
    expect(names).toEqual(['None (top level)', 'Grips', 'Wheels']) // not Tyres itself, not Tubes inside it
    fireEvent.change(parent, { target: { value: 'grips' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save category' }))
    expect(await screen.findByText('Saved.')).toBeInTheDocument()
    const put = find(calls, 'PUT', '/admin/categories/tyres')!
    expect(put.headers.get('If-Match')).toBe('"c1"')
    expect(put.body).toMatchObject({ parentId: 'grips', name: 'Tyres' })
  })

  it('switches an attribute to vary per variant', async () => {
    const { calls } = renderApp('/admin/categories/tyres', { ...api(), '/admin/categories/tyres/attributes/a-colour': new Reply(204) })
    fireEvent.click(await screen.findByRole('checkbox', { name: 'Colour varies per variant' }))
    await waitFor(() => expect(find(calls, 'PUT', '/admin/categories/tyres/attributes/a-colour')).toBeDefined())
    expect(find(calls, 'PUT', '/admin/categories/tyres/attributes/a-colour')!.body).toEqual({
      position: 0,
      isRequired: false,
      isVariantAxis: true,
      labelOverride: null,
    })
  })

  it('warns before removing an attribute, because that deletes product values', async () => {
    const { calls } = renderApp('/admin/categories/tyres', { ...api(), '/admin/categories/tyres/attributes/a-colour': new Reply(204) })
    const table = await screen.findByRole('table')
    fireEvent.click(within(table).getByRole('button', { name: 'Remove' }))
    expect(screen.getByText(/Removes its values from every product here/)).toBeInTheDocument()
    expect(find(calls, 'DELETE', '/admin/categories/tyres/attributes/a-colour')).toBeUndefined()
    fireEvent.click(within(table).getByRole('button', { name: 'Remove' }))
    await waitFor(() => expect(find(calls, 'DELETE', '/admin/categories/tyres/attributes/a-colour')).toBeDefined())
  })
})

describe('attributes', () => {
  it('derives the key from the name and opens the new attribute', async () => {
    const created = { ...colour, id: 'a-valve', key: 'valve_type', label: 'Valve type', dataType: 'enum' as const }
    const { calls, router } = renderApp('/admin/attributes', {
      ...api(),
      '/admin/attributes': (_u: URL, c: Call) =>
        c.method === 'POST' ? new Reply(201, created, { ETag: '"a1"' }) : { items: [colour] },
      '/admin/attributes/a-valve': new Reply(200, created, { ETag: '"a1"' }),
    })
    const form = await screen.findByRole('form', { name: 'New attribute' })
    fireEvent.change(within(form).getByLabelText('Name'), { target: { value: 'Valve type' } })
    expect(within(form).getByLabelText('Key')).toHaveValue('valve_type')
    fireEvent.click(within(form).getByRole('button', { name: 'Create attribute' }))
    await waitFor(() => expect(router.state.location.pathname).toBe('/admin/attributes/a-valve'))
    expect(find(calls, 'POST', '/admin/attributes')!.body).toMatchObject({ key: 'valve_type', label: 'Valve type', dataType: 'enum' })
  })

  it('makes readable keys and option values', () => {
    expect(keyFromLabel('Wheel size (in)')).toBe('wheel_size_in')
    expect(keyFromLabel('26 inch')).toBe('a_26_inch') // keys must start with a letter
    expect(optionValue('26 Inch')).toBe('26-inch')
    expect(optionValue('Crème Brûlée')).toBe('creme-brulee')
  })
})

describe('product photos', () => {
  function product(etagVersion: number, overrides: Partial<AdminProduct> = {}): Reply {
    const p: AdminProduct = {
      id: 'p1',
      categoryId: 'grips',
      category: { id: 'grips', name: 'Grips', slug: 'grips' },
      brandId: null,
      name: 'Star Grips',
      slug: 'star-grips',
      summary: null,
      description: null,
      status: 'active',
      isFeatured: false,
      retailPriceIsPublic: false,
      attributes: {},
      variants: [
        {
          id: 'v1',
          sku: 'SG',
          supplierId: null,
          supplierItemNo: null,
          modelNo: null,
          nameSuffix: null,
          position: 0,
          stockStatus: 'in_stock',
          isDefault: true,
          attributes: {},
          prices: {},
        },
      ],
      media: [],
      createdAt: '',
      updatedAt: '',
      publishedAt: null,
      ...overrides,
    }
    return new Reply(200, p, { ETag: `"v${etagVersion}"` })
  }

  const photo = {
    id: 'm1',
    assetId: 'asset1',
    variantId: null,
    role: 'hero',
    position: 0,
    altText: null,
    status: 'ready',
    image: { width: 10, height: 10, sources: [{ url: '/media/r/a.jpeg', width: 10, height: 10, format: 'jpeg' }] },
  }

  async function uploadPhoto(afterUpload: Reply) {
    let uploaded = false
    const rendered = renderApp('/admin/products/p1', {
      ...api(),
      '/admin/categories/grips/form-schema': { categoryId: 'grips', version: 1, fields: [] },
      '/admin/media': new Reply(201, { id: 'asset1', status: 'pending' }),
      '/admin/products/p1/media': () => {
        uploaded = true
        return new Reply(201, photo)
      },
      '/admin/products/p1': (_u: URL, c: Call) =>
        c.method === 'PUT' ? product(3) : uploaded ? afterUpload : product(1),
    })
    const input = await screen.findByLabelText('Add photos')
    fireEvent.change(input, { target: { files: [new File(['x'], 'grips.jpg', { type: 'image/jpeg' })] } })
    await screen.findByRole('combobox', { name: 'Use as' })
    return rendered
  }

  it('uploads, attaches as the main photo, and takes the new version for the next save', async () => {
    const { calls } = await uploadPhoto(product(2, { media: [photo as AdminProduct['media'][number]] }))
    expect(find(calls, 'POST', '/admin/products/p1/media')!.body).toMatchObject({ assetId: 'asset1', role: 'hero', position: 0 })
    expect(screen.getByRole('combobox', { name: 'Use as' })).toHaveValue('hero')

    fireEvent.change(screen.getByLabelText('Name', { exact: true }), { target: { value: 'Star Grips Pro' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    await waitFor(() => expect(find(calls, 'PUT', '/admin/products/p1')).toBeDefined())
    expect(find(calls, 'PUT', '/admin/products/p1')!.headers.get('If-Match')).toBe('"v2"')
  })

  it("keeps the old version if someone else edited the product meanwhile, so their edit isn't overwritten", async () => {
    const { calls } = await uploadPhoto(
      product(2, { name: 'Renamed by someone else', media: [photo as AdminProduct['media'][number]] }),
    )
    fireEvent.change(screen.getByLabelText('Name', { exact: true }), { target: { value: 'My name' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    await waitFor(() => expect(find(calls, 'PUT', '/admin/products/p1')).toBeDefined())
    expect(find(calls, 'PUT', '/admin/products/p1')!.headers.get('If-Match')).toBe('"v1"')
  })
})
