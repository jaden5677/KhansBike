// Data hooks for the public catalogue: one per API endpoint. Each query key
// contains every parameter the request depends on, so TanStack Query caches
// each filter combination separately and going back to a view is instant.

import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { api } from './client'
import type {
  CategoryNode,
  CategoryPage,
  Facet,
  Page,
  ProductCard,
  ProductDetail,
  PublicBrand,
  SearchGroup,
  Suggestion,
} from './types'

interface Items<T> {
  items: T[]
}

export function useCategoryTree() {
  return useQuery({
    queryKey: ['categories'],
    queryFn: ({ signal }) => api.get<Items<CategoryNode>>('/categories', undefined, signal).then((r) => r.items),
  })
}

export function useCategory(slug: string) {
  return useQuery({
    queryKey: ['category', slug],
    queryFn: ({ signal }) => api.get<CategoryPage>(`/categories/${encodeURIComponent(slug)}`, undefined, signal),
  })
}

/** Filter options for a category under the current filters (a listingQuery). */
export function useFacets(slug: string, query: URLSearchParams) {
  return useQuery({
    queryKey: ['facets', slug, query.toString()],
    queryFn: ({ signal }) =>
      api.get<Items<Facet>>(`/categories/${encodeURIComponent(slug)}/facets`, query, signal).then((r) => r.items),
    // Keep showing the old options while new counts load, so the panel
    // does not flash empty on every click.
    placeholderData: (previous) => previous,
  })
}

/**
 * A product listing that grows page by page: fetchNextPage() asks for the
 * page after the last one using the API's opaque nextCursor.
 */
export function useProductList(query: URLSearchParams, limit = 24) {
  return useInfiniteQuery({
    queryKey: ['products', query.toString(), limit],
    initialPageParam: '',
    queryFn: ({ pageParam, signal }) => {
      const q = new URLSearchParams(query)
      q.set('limit', String(limit))
      if (pageParam) q.set('cursor', pageParam)
      return api.get<Page<ProductCard>>('/products', q, signal)
    },
    getNextPageParam: (last) => last.nextCursor || undefined,
  })
}

export function useProduct(slug: string) {
  return useQuery({
    queryKey: ['product', slug],
    queryFn: ({ signal }) => api.get<ProductDetail>(`/products/${encodeURIComponent(slug)}`, undefined, signal),
  })
}

export function useSearch(text: string) {
  const q = text.trim()
  return useQuery({
    queryKey: ['search', q],
    queryFn: ({ signal }) => api.get<{ groups: SearchGroup[] }>('/search', { q }, signal).then((r) => r.groups),
    enabled: q !== '',
  })
}

/** Type-ahead names; the API answers nothing below two characters. */
export function useSuggestions(text: string) {
  const q = text.trim()
  return useQuery({
    queryKey: ['suggest', q],
    queryFn: ({ signal }) => api.get<Items<Suggestion>>('/search/suggest', { q }, signal).then((r) => r.items),
    enabled: q.length >= 2,
    placeholderData: (previous) => previous,
  })
}

export function useFitment(wheelSize: string) {
  return useQuery({
    queryKey: ['fitment', wheelSize],
    queryFn: ({ signal }) =>
      api
        .get<{ groups: SearchGroup[] }>(`/fitment/${encodeURIComponent(wheelSize)}`, undefined, signal)
        .then((r) => r.groups),
  })
}

export function useBrands() {
  return useQuery({
    queryKey: ['brands'],
    queryFn: ({ signal }) => api.get<Items<PublicBrand>>('/brands', undefined, signal).then((r) => r.items),
  })
}
