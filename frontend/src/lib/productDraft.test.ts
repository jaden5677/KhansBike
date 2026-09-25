import { describe, expect, it } from 'vitest'
import type { AdminProduct, FormSchema } from '../api/adminTypes'
import { ApiError } from '../api/client'
import { draftFromProduct, emptyDraft, fieldErrors, toInput } from './productDraft'

const product: AdminProduct = {
  id: 'p1',
  categoryId: 'c1',
  category: { id: 'c1', name: 'Grips', slug: 'grips' },
  brandId: null,
  name: 'Star Grips',
  slug: 'star-grips',
  summary: null,
  description: 'Soft rubber.',
  status: 'active',
  isFeatured: false,
  retailPriceIsPublic: true,
  attributes: { material: 'rubber' },
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

const schema: FormSchema = {
  categoryId: 'c1',
  version: 1,
  fields: [
    { key: 'material', label: 'Material', inputType: 'text', dataType: 'text', required: false, isVariantAxis: false },
    { key: 'colour', label: 'Colour', inputType: 'select', dataType: 'enum', required: true, isVariantAxis: true },
  ],
}

describe('product draft', () => {
  it('round-trips a loaded product into the same save body', () => {
    const input = toInput(draftFromProduct(product), schema)
    expect(input).toEqual({
      categoryId: 'c1',
      brandId: null,
      name: 'Star Grips',
      slug: 'star-grips',
      summary: null,
      description: 'Soft rubber.',
      status: 'active',
      isFeatured: false,
      retailPriceIsPublic: true,
      attributes: { material: 'rubber' },
      variants: [
        {
          id: 'v1',
          sku: 'SG-BK',
          supplierId: null,
          supplierItemNo: null,
          modelNo: null,
          nameSuffix: null,
          stockStatus: 'in_stock',
          isDefault: true,
          attributes: { colour: 'black' },
          prices: { retail_ttd: '45.00' },
        },
      ],
    })
  })

  it('drops empty values, blank prices, and attributes the category does not define at that level', () => {
    const draft = emptyDraft('c1')
    draft.name = '  New grips '
    draft.attributes = { material: '', colour: 'red', stale: 'x' }
    draft.variants[0].attributes = { colour: 'red', material: 'foam', sizes: [] }
    draft.variants[0].prices = { retail_ttd: ' 50 ', cost_usd: '' }
    const input = toInput(draft, schema)
    expect(input.name).toBe('New grips')
    expect(input.slug).toBeUndefined() // derived by the server
    expect(input.attributes).toEqual({})
    expect(input.variants[0]).toMatchObject({ attributes: { colour: 'red' }, prices: { retail_ttd: '50' } })
    expect(input.variants[0]).not.toHaveProperty('id')
  })

  it('keeps every filled-in value while the schema is still loading', () => {
    const draft = emptyDraft('c1')
    draft.attributes = { material: 'rubber' }
    expect(toInput(draft).attributes).toEqual({ material: 'rubber' })
  })

  it('indexes 422 messages by field path', () => {
    const err = new ApiError(422, 'Validation failed', undefined, [
      { field: 'variants[0].sku', message: 'duplicates another variant' },
    ])
    expect(fieldErrors(err)).toEqual({ 'variants[0].sku': 'duplicates another variant' })
    expect(fieldErrors(new Error('x'))).toEqual({})
  })
})
