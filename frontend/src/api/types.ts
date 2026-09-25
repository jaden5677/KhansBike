// TypeScript mirrors of the JSON the Go API returns (internal/http/dto).
// Keep these in step with the Go structs: the field names come from their
// `json:"..."` tags. Admin types are added alongside the admin screens.

/** A list page; pass nextCursor back as ?cursor= to get the next one. */
export interface Page<T> {
  items: T[]
  total?: number
  nextCursor?: string
}

/** Money is a decimal string, never a float, so no rounding surprises. */
export interface Money {
  amount: string
  currency: 'TTD' | 'USD'
}

export type StockStatus = 'in_stock' | 'low' | 'out' | 'special_order' | 'unknown'

export type DataType =
  | 'text'
  | 'number'
  | 'number_range'
  | 'boolean'
  | 'enum'
  | 'multi_enum'
  | 'color'

export interface BrandRef {
  id: string
  name: string
  slug: string
}

export interface CategoryRef {
  id: string
  name: string
  slug: string
}

/** One encoded rendition of an image (WebP or JPEG at one width). */
export interface ImageSource {
  url: string
  width: number
  height: number
  format: 'webp' | 'jpeg'
}

export interface Image {
  width: number
  height: number
  blurhash?: string
  dominantHex?: string
  altText?: string
  sources: ImageSource[]
}

export interface ProductCard {
  id: string
  slug: string
  name: string
  summary?: string
  brand?: BrandRef
  category: CategoryRef
  isFeatured: boolean
  /** Null when the price is not public: show "ask in store" instead. */
  fromPrice: Money | null
  availability: StockStatus
  variantCount: number
  image: Image | null
}

export interface AttributeValue {
  key: string
  label: string
  dataType: DataType
  unit?: string
  value: unknown
  /** Ready-to-show text, e.g. "26 in" or "Red, Black". */
  display: string
  swatchHex?: string
}

export interface Variant {
  id: string
  sku: string
  modelNo?: string
  nameSuffix?: string
  stockStatus: StockStatus
  isDefault: boolean
  attributes: AttributeValue[]
  price: Money | null
}

export type MediaRole = 'hero' | 'gallery' | 'detail' | 'swatch'

export interface ProductImage extends Image {
  id: string
  role: MediaRole
  variantId?: string
}

export interface ProductDetail {
  id: string
  slug: string
  name: string
  summary?: string
  description?: string
  brand?: BrandRef
  category: CategoryRef
  isFeatured: boolean
  publishedAt?: string
  attributes: AttributeValue[]
  variants: Variant[]
  images: ProductImage[]
}

export interface CategoryNode {
  id: string
  name: string
  slug: string
  description?: string
  productCount: number
  image: Image | null
  children: CategoryNode[]
}

export interface CategoryPage extends CategoryNode {
  breadcrumbs: CategoryRef[]
}

export interface FacetValue {
  value: string
  label: string
  count: number
}

export interface Facet {
  key: string
  label: string
  dataType: DataType
  values?: FacetValue[]
  numMin?: number
  numMax?: number
}

export interface SearchGroup {
  category: CategoryRef
  total: number
  products: ProductCard[]
}

export interface Suggestion {
  slug: string
  name: string
}

export interface PublicBrand {
  id: string
  name: string
  slug: string
  logo: Image | null
}

export interface User {
  id: string
  email: string
  displayName: string
  role: 'admin' | 'wholesale'
}

/** GET /auth/session and POST /auth/login. */
export interface Session {
  user: User
  /** "admin" is a browser session; "device" is a paired phone. */
  kind: 'admin' | 'device'
  csrfToken?: string
  expiresAt?: string
}
