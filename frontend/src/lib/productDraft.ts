// The product editor's data model. The API saves a product as one document
// (fields, attribute values, variants, prices), so the editor holds exactly
// one "draft" of that document and converts it back to the API's input
// shape on save. These are pure functions: no React, easy to test.

import type {
  AdminProduct,
  AttrValue,
  FormSchema,
  PriceTier,
  ProductInput,
  ProductStatus,
  VariantInput,
} from '../api/adminTypes'
import { ApiError } from '../api/client'
import type { StockStatus } from '../api/types'

export interface VariantDraft {
  /** A stable React key; new variants have no id yet. */
  key: string
  id?: string
  sku: string
  supplierId: string
  supplierItemNo: string
  modelNo: string
  nameSuffix: string
  stockStatus: StockStatus
  isDefault: boolean
  attributes: Record<string, AttrValue>
  /** Decimal strings as typed, e.g. "85.00"; "" = keep/unset. */
  prices: Partial<Record<PriceTier, string>>
}

export interface ProductDraft {
  categoryId: string
  brandId: string
  name: string
  slug: string
  summary: string
  description: string
  status: ProductStatus
  isFeatured: boolean
  retailPriceIsPublic: boolean
  attributes: Record<string, AttrValue>
  variants: VariantDraft[]
}

let nextKey = 0
const newKey = () => `new-${++nextKey}`

export function emptyVariant(isDefault = false): VariantDraft {
  return {
    key: newKey(),
    sku: '',
    supplierId: '',
    supplierItemNo: '',
    modelNo: '',
    nameSuffix: '',
    stockStatus: 'unknown',
    isDefault,
    attributes: {},
    prices: {},
  }
}

export function emptyDraft(categoryId = ''): ProductDraft {
  return {
    categoryId,
    brandId: '',
    name: '',
    slug: '',
    summary: '',
    description: '',
    status: 'draft',
    isFeatured: false,
    retailPriceIsPublic: false,
    attributes: {},
    variants: [emptyVariant(true)],
  }
}

export function draftFromProduct(p: AdminProduct): ProductDraft {
  return {
    categoryId: p.categoryId,
    brandId: p.brandId ?? '',
    name: p.name,
    slug: p.slug,
    summary: p.summary ?? '',
    description: p.description ?? '',
    status: p.status,
    isFeatured: p.isFeatured,
    retailPriceIsPublic: p.retailPriceIsPublic,
    attributes: { ...p.attributes },
    variants: p.variants.map((v) => ({
      key: v.id,
      id: v.id,
      sku: v.sku,
      supplierId: v.supplierId ?? '',
      supplierItemNo: v.supplierItemNo ?? '',
      modelNo: v.modelNo ?? '',
      nameSuffix: v.nameSuffix ?? '',
      stockStatus: v.stockStatus,
      isDefault: v.isDefault,
      attributes: { ...v.attributes },
      prices: Object.fromEntries(Object.entries(v.prices).map(([tier, price]) => [tier, price?.amount ?? ''])),
    })),
  }
}

const orNull = (s: string) => (s.trim() === '' ? null : s.trim())

/** Whether an attribute value counts as "filled in". */
export function hasValue(v: AttrValue | undefined): boolean {
  if (v === null || v === undefined || v === '') return false
  if (Array.isArray(v)) return v.length > 0
  return true
}

/**
 * Keeps the filled-in values whose key the category defines at this level.
 * Without a schema (still loading) every filled-in value is kept.
 */
function pickAttributes(
  values: Record<string, AttrValue>,
  schema: FormSchema | undefined,
  variantLevel: boolean,
): Record<string, AttrValue> {
  const allowed = schema
    ? new Set(schema.fields.filter((f) => f.isVariantAxis === variantLevel).map((f) => f.key))
    : undefined
  return Object.fromEntries(
    Object.entries(values).filter(([key, value]) => hasValue(value) && (!allowed || allowed.has(key))),
  )
}

/** The request body for a save. */
export function toInput(d: ProductDraft, schema?: FormSchema): ProductInput {
  return {
    categoryId: d.categoryId,
    brandId: orNull(d.brandId),
    name: d.name.trim(),
    slug: d.slug.trim() || undefined, // derived from the name when empty
    summary: orNull(d.summary),
    description: orNull(d.description),
    status: d.status,
    isFeatured: d.isFeatured,
    retailPriceIsPublic: d.retailPriceIsPublic,
    attributes: pickAttributes(d.attributes, schema, false),
    variants: d.variants.map(
      (v): VariantInput => ({
        ...(v.id ? { id: v.id } : {}),
        sku: v.sku.trim(),
        supplierId: orNull(v.supplierId),
        supplierItemNo: orNull(v.supplierItemNo),
        modelNo: orNull(v.modelNo),
        nameSuffix: orNull(v.nameSuffix),
        stockStatus: v.stockStatus,
        isDefault: v.isDefault,
        attributes: pickAttributes(v.attributes, schema, true),
        // An omitted tier keeps its current price on the server.
        prices: Object.fromEntries(
          Object.entries(v.prices)
            .map(([tier, amount]) => [tier, (amount ?? '').trim()])
            .filter(([, amount]) => amount !== ''),
        ),
      }),
    ),
  }
}

/** The server's 422 field messages, keyed by field path (e.g. "variants[0].sku"). */
export function fieldErrors(error: unknown): Record<string, string> {
  if (!(error instanceof ApiError)) return {}
  return Object.fromEntries(error.fieldErrors.map((e) => [e.field, e.message]))
}
