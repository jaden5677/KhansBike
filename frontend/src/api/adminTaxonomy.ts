// Data hooks for the catalogue structure (categories, attributes, options,
// brands, suppliers) and for product photos.

import { useMutation, useQuery, useQueryClient, type QueryKey } from '@tanstack/react-query'
import type { Versioned } from './admin'
import type {
  AdminAttribute,
  AdminCategory,
  AdminMedia,
  Asset,
  AttributeInput,
  Binding,
  BindingInput,
  CategoryInput,
  MediaInput,
  OptionInput,
} from './adminTypes'
import { api, request } from './client'

/**
 * A write plus "these cached answers are now out of date". Every structure
 * edit is exactly that, so each screen only states the request and what it
 * affects; the invalidated queries refetch when next shown.
 */
function useAdminMutation<TInput, TResult>(fn: (input: TInput) => Promise<TResult>, affects: QueryKey[]) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: fn,
    onSuccess: () => Promise.all(affects.map((queryKey) => queryClient.invalidateQueries({ queryKey }))),
  })
}

/**
 * Like useAdminMutation, but the saved record (with its new ETag) goes into
 * the cache at once. Otherwise, until the refetch lands, the form would still
 * hold the old ETag and a quick second save would be refused as a conflict.
 */
function useVersionedSave<TInput, T extends { id: string }>(
  fn: (input: TInput) => Promise<Versioned<T>>,
  detailKey: string,
) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: fn,
    onSuccess: (saved) => {
      queryClient.setQueryData(['admin', detailKey, saved.data.id], saved)
      return Promise.all(
        structure
          .filter((key) => key[1] !== detailKey)
          .map((queryKey) => queryClient.invalidateQueries({ queryKey })),
      )
    },
  })
}

// Whatever a structure change can alter: lists, details and the product
// form, whose fields come from the category's attributes.
const structure: QueryKey[] = [
  ['admin', 'categories'],
  ['admin', 'category'],
  ['admin', 'bindings'],
  ['admin', 'attributes'],
  ['admin', 'attribute'],
  ['admin', 'form-schema'],
]

// ---- categories ----

export function useAdminCategory(id: string) {
  return useQuery({
    queryKey: ['admin', 'category', id],
    queryFn: ({ signal }) =>
      request<AdminCategory>('GET', `/admin/categories/${id}`, { signal }) as Promise<Versioned<AdminCategory>>,
  })
}

export function useSaveCategory() {
  return useVersionedSave(
    ({ id, input, etag }: { id?: string; input: CategoryInput; etag?: string }) =>
      id
        ? request<AdminCategory>('PUT', `/admin/categories/${id}`, { body: input, ifMatch: etag })
        : request<AdminCategory>('POST', '/admin/categories', { body: input }),
    'category',
  )
}

/**
 * Deletes a record. Its own cached details are only marked stale, not
 * refetched: the page showing them is about to navigate away, and asking now
 * would just get a 404 (a later visit refetches and shows "not found").
 * The lists that showed it are refetched.
 */
function useDelete(path: string, detailKeys: string[]) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => api.delete(`${path}/${id}`),
    onSuccess: (_result, id) => {
      for (const key of detailKeys) {
        void queryClient.invalidateQueries({ queryKey: ['admin', key, id], refetchType: 'none' })
      }
      return Promise.all(
        structure
          .filter((key) => !detailKeys.includes(key[1] as string))
          .map((queryKey) => queryClient.invalidateQueries({ queryKey })),
      )
    },
  })
}

export function useDeleteCategory() {
  return useDelete('/admin/categories', ['category', 'bindings'])
}

export function useBindings(categoryId: string) {
  return useQuery({
    queryKey: ['admin', 'bindings', categoryId],
    queryFn: ({ signal }) =>
      api.get<{ items: Binding[] }>(`/admin/categories/${categoryId}/attributes`, undefined, signal).then((r) => r.items),
  })
}

/**
 * Attaches an attribute or changes its settings, optimistically: the
 * checkbox shows the new setting at once instead of waiting for the round
 * trip. If the server refuses (e.g. 409: products already hold values, so
 * "varies per variant" cannot change), the old list is put back.
 */
export function useBindAttribute(categoryId: string) {
  const queryClient = useQueryClient()
  const key = ['admin', 'bindings', categoryId]
  return useMutation({
    mutationFn: ({ attributeId, input }: { attributeId: string; input: BindingInput }) =>
      api.put<void>(`/admin/categories/${categoryId}/attributes/${attributeId}`, input),
    onMutate: async ({ attributeId, input }) => {
      await queryClient.cancelQueries({ queryKey: key }) // a late refetch must not undo the change
      const previous = queryClient.getQueryData<Binding[]>(key)
      queryClient.setQueryData<Binding[]>(key, (list) =>
        list?.map((b) => (b.attribute.id === attributeId ? { ...b, ...input } : b)),
      )
      return { previous }
    },
    onError: (_error, _input, context) => {
      if (context?.previous) queryClient.setQueryData(key, context.previous)
    },
    onSettled: () => Promise.all(structure.map((queryKey) => queryClient.invalidateQueries({ queryKey }))),
  })
}

export function useUnbindAttribute(categoryId: string) {
  return useAdminMutation(
    (attributeId: string) => api.delete(`/admin/categories/${categoryId}/attributes/${attributeId}`),
    // Unbinding deletes the attribute's values from the category's products.
    [...structure, ['admin', 'product']],
  )
}

// ---- attributes and options ----

export function useAdminAttributes() {
  return useQuery({
    queryKey: ['admin', 'attributes'],
    queryFn: ({ signal }) => api.get<{ items: AdminAttribute[] }>('/admin/attributes', undefined, signal).then((r) => r.items),
  })
}

export function useAdminAttribute(id: string) {
  return useQuery({
    queryKey: ['admin', 'attribute', id],
    queryFn: ({ signal }) =>
      request<AdminAttribute>('GET', `/admin/attributes/${id}`, { signal }) as Promise<Versioned<AdminAttribute>>,
  })
}

export function useSaveAttribute() {
  return useVersionedSave(
    ({ id, input, etag }: { id?: string; input: AttributeInput; etag?: string }) =>
      id
        ? request<AdminAttribute>('PUT', `/admin/attributes/${id}`, { body: input, ifMatch: etag })
        : request<AdminAttribute>('POST', '/admin/attributes', { body: input }),
    'attribute',
  )
}

export function useDeleteAttribute() {
  return useDelete('/admin/attributes', ['attribute'])
}

export function useSaveOption(attributeId: string) {
  return useAdminMutation(
    ({ optionId, input }: { optionId?: string; input: OptionInput }) =>
      optionId
        ? api.put<void>(`/admin/attributes/${attributeId}/options/${optionId}`, input)
        : api.post<void>(`/admin/attributes/${attributeId}/options`, input),
    structure,
  )
}

export function useDeleteOption(attributeId: string) {
  return useAdminMutation(
    (optionId: string) => api.delete(`/admin/attributes/${attributeId}/options/${optionId}`),
    structure,
  )
}

// ---- brands and suppliers ----

export function useSaveBrand() {
  return useAdminMutation(
    ({ id, input }: { id?: string; input: { name: string; slug?: string; position: number } }) =>
      id ? api.put<void>(`/admin/brands/${id}`, input) : api.post<void>('/admin/brands', input),
    [['admin', 'brands']],
  )
}

export function useDeleteBrand() {
  return useAdminMutation((id: string) => api.delete(`/admin/brands/${id}`), [['admin', 'brands']])
}

export function useSaveSupplier() {
  return useAdminMutation(
    ({ id, input }: { id?: string; input: { name: string; code: string | null; notes: string | null } }) =>
      id ? api.put<void>(`/admin/suppliers/${id}`, input) : api.post<void>('/admin/suppliers', input),
    [['admin', 'suppliers']],
  )
}

export function useDeleteSupplier() {
  return useAdminMutation((id: string) => api.delete(`/admin/suppliers/${id}`), [['admin', 'suppliers']])
}

// ---- photos ----

/** Uploads one image; the server deduplicates identical files. */
export function uploadImage(file: File): Promise<Asset> {
  const form = new FormData()
  form.append('file', file)
  return api.post<Asset>('/admin/media', form)
}

export function attachMedia(productId: string, input: MediaInput): Promise<AdminMedia> {
  return api.post<AdminMedia>(`/admin/products/${productId}/media`, input)
}

export function updateMedia(productId: string, mediaId: string, input: MediaInput): Promise<AdminMedia> {
  return api.put<AdminMedia>(`/admin/products/${productId}/media/${mediaId}`, input)
}

export function detachMedia(productId: string, mediaId: string): Promise<void> {
  return api.delete(`/admin/products/${productId}/media/${mediaId}`)
}
