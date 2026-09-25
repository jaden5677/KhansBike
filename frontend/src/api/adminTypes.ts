// TypeScript mirrors of the admin API's JSON (internal/http/dto/admin.go).
import type { DataType, Image, ProductCard, StockStatus } from './types'

export type ProductStatus = 'draft' | 'active' | 'discontinued' | 'needs_review'
export type PriceTier = 'cost_usd' | 'landed_ttd' | 'wholesale_ttd' | 'retail_ttd'

export const priceTiers: { tier: PriceTier; label: string }[] = [
  { tier: 'cost_usd', label: 'Cost (USD)' },
  { tier: 'landed_ttd', label: 'Landed (TTD)' },
  { tier: 'wholesale_ttd', label: 'Wholesale (TTD)' },
  { tier: 'retail_ttd', label: 'Retail (TTD)' },
]

export const statusLabels: Record<ProductStatus, string> = {
  draft: 'Draft',
  active: 'Active',
  discontinued: 'Discontinued',
  needs_review: 'Needs review',
}

export interface AdminProductSummary extends ProductCard {
  status: ProductStatus
  updatedAt: string
}

export interface AdminPrice {
  amount: string
  currency: string
  effectiveFrom: string
}

/**
 * An attribute value in its JSON shape (see internal/service/schema.go):
 * text "x", number 26, number_range {low, high}, boolean true,
 * enum/color "black", multi_enum ["road", "gravel"].
 */
export type AttrValue = string | number | boolean | string[] | { low: number; high: number } | null

export interface AdminVariant {
  id: string
  sku: string
  supplierId: string | null
  supplierItemNo: string | null
  modelNo: string | null
  nameSuffix: string | null
  position: number
  stockStatus: StockStatus
  isDefault: boolean
  attributes: Record<string, AttrValue>
  prices: Partial<Record<PriceTier, AdminPrice>>
}

export interface AdminMedia {
  id: string
  assetId: string
  variantId: string | null
  role: string
  position: number
  altText: string | null
  status: string
  image: Image | null
}

export interface AdminProduct {
  id: string
  categoryId: string
  category: { id: string; name: string; slug: string }
  brandId: string | null
  name: string
  slug: string
  summary: string | null
  description: string | null
  status: ProductStatus
  isFeatured: boolean
  retailPriceIsPublic: boolean
  attributes: Record<string, AttrValue>
  variants: AdminVariant[]
  media: AdminMedia[]
  createdAt: string
  updatedAt: string
  publishedAt: string | null
}

/** The write shape: POST /admin/products and PUT /admin/products/{id}. */
export interface ProductInput {
  categoryId: string
  brandId: string | null
  name: string
  slug?: string
  summary: string | null
  description: string | null
  status: ProductStatus
  isFeatured: boolean
  retailPriceIsPublic: boolean
  attributes: Record<string, AttrValue>
  variants: VariantInput[]
}

export interface VariantInput {
  id?: string
  sku: string
  supplierId: string | null
  supplierItemNo: string | null
  modelNo: string | null
  nameSuffix: string | null
  stockStatus: StockStatus
  isDefault: boolean
  attributes: Record<string, AttrValue>
  prices: Partial<Record<PriceTier, string>>
}

export interface AdminCategory {
  id: string
  parentId: string | null
  name: string
  slug: string
  path: string
  position: number
  isActive: boolean
}

export interface AdminBrand {
  id: string
  name: string
  slug: string
}

export interface AdminSupplier {
  id: string
  name: string
  code: string | null
}

export interface FormOption {
  value: string
  label: string
  swatchHex?: string
}

export interface FormField {
  key: string
  label: string
  inputType: string
  dataType: DataType
  unit?: string
  required: boolean
  isVariantAxis: boolean
  helpText?: string
  options?: FormOption[]
  min?: number
  max?: number
}

export interface FormSchema {
  categoryId: string
  version: number
  fields: FormField[]
}
