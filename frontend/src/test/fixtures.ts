// Small, realistic API responses shared by the page tests.
import type { CategoryPage, Facet, Page, ProductCard, ProductDetail, SearchGroup } from '../api/types'

const tyres = { id: 'cat-tyres', name: 'Tyres', slug: 'tyres' }

export const riderCard: ProductCard = {
  id: 'p-rider',
  slug: 'rider-20',
  name: 'Rider 20',
  category: tyres,
  brand: { id: 'b-kenda', name: 'Kenda', slug: 'kenda' },
  isFeatured: true,
  fromPrice: { amount: '85.00', currency: 'TTD' },
  availability: 'in_stock',
  variantCount: 2,
  image: null,
}

export const trailCard: ProductCard = {
  ...riderCard,
  id: 'p-trail',
  slug: 'trail-26',
  name: 'Trail 26',
  fromPrice: null,
  availability: 'low',
  variantCount: 1,
}

export const productPage: Page<ProductCard> = { items: [riderCard, trailCard], total: 2 }

export const tyresCategory: CategoryPage = {
  ...tyres,
  description: 'Tyres for every wheel.',
  productCount: 2,
  image: null,
  children: [],
  breadcrumbs: [{ id: 'cat-wheels', name: 'Wheels', slug: 'wheels' }],
}

export const tyreFacets: Facet[] = [
  {
    key: 'colour',
    label: 'Colour',
    dataType: 'enum',
    values: [
      { value: 'red', label: 'Red', count: 1 },
      { value: 'black', label: 'Black', count: 2 },
    ],
  },
  { key: 'width', label: 'Width', dataType: 'number', numMin: 1.5, numMax: 2.4 },
]

export const riderDetail: ProductDetail = {
  id: 'p-rider',
  slug: 'rider-20',
  name: 'Rider 20',
  summary: 'A tough 20-inch tyre.',
  description: 'Line one.\nLine two.',
  brand: riderCard.brand,
  category: tyres,
  isFeatured: true,
  attributes: [
    { key: 'wheel_size', label: 'Wheel size', dataType: 'enum', value: '20', display: '20 in' },
  ],
  variants: [
    {
      id: 'v-red',
      sku: 'RID-20-RED',
      stockStatus: 'in_stock',
      isDefault: true,
      attributes: [{ key: 'colour', label: 'Colour', dataType: 'enum', value: 'red', display: 'Red' }],
      price: { amount: '85.00', currency: 'TTD' },
    },
    {
      id: 'v-black',
      sku: 'RID-20-BLK',
      stockStatus: 'out',
      isDefault: false,
      attributes: [{ key: 'colour', label: 'Colour', dataType: 'enum', value: 'black', display: 'Black' }],
      price: null,
    },
  ],
  images: [],
}

export const searchGroups: SearchGroup[] = [{ category: tyres, total: 5, products: [riderCard, trailCard] }]
