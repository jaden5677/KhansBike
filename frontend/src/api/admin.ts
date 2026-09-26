// Data hooks for the admin screens. Every key starts with 'admin', so
// signing out can drop all of them at once (see auth/session.ts).

import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type {
  AdminBrand,
  AdminCategory,
  AdminProduct,
  AdminProductSummary,
  AdminSupplier,
  FormSchema,
  ProductInput,
} from './adminTypes'
import { api, request } from './client'
import type { Page } from './types'

interface Items<T> {
  items: T[]
}

/** The admin product list; `query` takes the same filters as the public one plus status. */
export function useAdminProducts(query: URLSearchParams, limit = 50) {
  return useInfiniteQuery({
    queryKey: ['admin', 'products', query.toString(), limit],
    initialPageParam: '',
    queryFn: ({ pageParam, signal }) => {
      const q = new URLSearchParams(query)
      q.set('limit', String(limit))
      if (pageParam) q.set('cursor', pageParam)
      return api.get<Page<AdminProductSummary>>('/admin/products', q, signal)
    },
    getNextPageParam: (last) => last.nextCursor || undefined,
  })
}

/** How many products have a status (one tiny request each). */
export function useProductCount(status: string) {
  return useQuery({
    queryKey: ['admin', 'product-count', status],
    queryFn: ({ signal }) =>
      api.get<Page<AdminProductSummary>>('/admin/products', { status, limit: 1 }, signal).then((p) => p.total ?? 0),
  })
}

/** A product with its ETag, which a save must send back as If-Match. */
export interface Versioned<T> {
  data: T
  etag?: string
}

export function useAdminProduct(id: string | undefined) {
  return useQuery({
    queryKey: ['admin', 'product', id],
    queryFn: ({ signal }) => request<AdminProduct>('GET', `/admin/products/${id}`, { signal }) as Promise<Versioned<AdminProduct>>,
    enabled: id !== undefined,
    // Never refetch in the background: that would swap the ETag under an
    // open editor and hide someone else's edit from the conflict check.
    staleTime: Infinity,
  })
}

export function useSaveProduct() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, input, etag }: { id?: string; input: ProductInput; etag?: string }) =>
      id
        ? request<AdminProduct>('PUT', `/admin/products/${id}`, { body: input, ifMatch: etag })
        : request<AdminProduct>('POST', '/admin/products', { body: input }),
    onSuccess: (saved) => {
      queryClient.setQueryData(['admin', 'product', saved.data.id], saved)
      // Lists and counts now show stale names/statuses; refetch when next shown.
      queryClient.invalidateQueries({ queryKey: ['admin', 'products'] })
      queryClient.invalidateQueries({ queryKey: ['admin', 'product-count'] })
    },
  })
}

export function useDeleteProduct() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => api.delete(`/admin/products/${id}`),
    onSuccess: (_, id) => {
      queryClient.removeQueries({ queryKey: ['admin', 'product', id] })
      queryClient.invalidateQueries({ queryKey: ['admin', 'products'] })
      queryClient.invalidateQueries({ queryKey: ['admin', 'product-count'] })
    },
  })
}

export function useAdminCategories() {
  return useQuery({
    queryKey: ['admin', 'categories'],
    queryFn: ({ signal }) => api.get<Items<AdminCategory>>('/admin/categories', undefined, signal).then((r) => r.items),
  })
}

export function useAdminBrands() {
  return useQuery({
    queryKey: ['admin', 'brands'],
    queryFn: ({ signal }) => api.get<Items<AdminBrand>>('/admin/brands', undefined, signal).then((r) => r.items),
  })
}

export function useAdminSuppliers() {
  return useQuery({
    queryKey: ['admin', 'suppliers'],
    queryFn: ({ signal }) => api.get<Items<AdminSupplier>>('/admin/suppliers', undefined, signal).then((r) => r.items),
  })
}

/** The fields a category's products have; drives the product editor. */
export function useFormSchema(categoryId: string) {
  return useQuery({
    queryKey: ['admin', 'form-schema', categoryId],
    queryFn: ({ signal }) => api.get<FormSchema>(`/admin/categories/${categoryId}/form-schema`, undefined, signal),
    enabled: categoryId !== '',
  })
}

export function useSubscriberStats() {
  return useQuery({
    queryKey: ['admin', 'subscriber-stats'],
    queryFn: ({ signal }) => api.get<Record<string, number>>('/admin/subscribers/stats', undefined, signal),
  })
}
