// Data hooks for the workbook importer: stage a workbook, review its rows,
// then commit or abort the batch.

import { useInfiniteQuery, useMutation, useQuery, useQueryClient, type InfiniteData } from '@tanstack/react-query'
import type { ImportBatch, ImportDecision, ImportRow } from './adminTypes'
import { api } from './client'
import type { Page } from './types'

export function useImportBatches() {
  return useQuery({
    queryKey: ['admin', 'imports'],
    queryFn: ({ signal }) => api.get<{ items: ImportBatch[] }>('/admin/imports', undefined, signal).then((r) => r.items),
  })
}

export function useImportBatch(id: string) {
  return useQuery({
    queryKey: ['admin', 'import', id],
    queryFn: ({ signal }) => api.get<ImportBatch>(`/admin/imports/${id}`, undefined, signal),
  })
}

export interface RowFilter {
  decision?: ImportDecision
  issuesOnly?: boolean
}

export function useImportRows(batchId: string, filter: RowFilter) {
  return useInfiniteQuery({
    queryKey: ['admin', 'import-rows', batchId, filter.decision ?? '', filter.issuesOnly ?? false],
    initialPageParam: '',
    queryFn: ({ pageParam, signal }) =>
      api.get<Page<ImportRow>>(
        `/admin/imports/${batchId}/rows`,
        { decision: filter.decision, issues: filter.issuesOnly ? 'true' : undefined, cursor: pageParam, limit: 100 },
        signal,
      ),
    getNextPageParam: (last) => last.nextCursor || undefined,
  })
}

/** Uploads a workbook; the server stages it as a dry run that changes nothing yet. */
export function useStageWorkbook() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (file: File) => {
      const form = new FormData()
      form.append('file', file)
      return api.post<ImportBatch>('/admin/imports', form)
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['admin', 'imports'] }),
  })
}

/**
 * Sets one row's decision. The changed row is replaced where it sits in the
 * loaded pages instead of reloading the list, so rows do not jump away while
 * someone works through them; only the batch counts are refetched.
 */
export function useDecideRow(batchId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ rowId, decision }: { rowId: string; decision: ImportDecision }) =>
      api.put<ImportRow>(`/admin/imports/${batchId}/rows/${rowId}`, { decision }),
    onSuccess: (row) => {
      queryClient.setQueriesData<InfiniteData<Page<ImportRow>>>({ queryKey: ['admin', 'import-rows', batchId] }, (data) =>
        data && {
          ...data,
          pages: data.pages.map((page) => ({ ...page, items: page.items.map((r) => (r.id === row.id ? row : r)) })),
        },
      )
      return queryClient.invalidateQueries({ queryKey: ['admin', 'import', batchId] })
    },
  })
}

/**
 * Skips every row still waiting for a decision. Those are the rows with
 * errors, which the server will not accept; the other way out is to fix the
 * workbook and import it again. Reads every waiting row first, then skips
 * them one by one, reporting progress.
 */
async function skipAllPending(batchId: string, onProgress: (done: number, total: number) => void): Promise<void> {
  const ids: string[] = []
  let cursor = ''
  do {
    const page = await api.get<Page<ImportRow>>(`/admin/imports/${batchId}/rows`, {
      decision: 'pending',
      cursor,
      limit: 200,
    })
    ids.push(...page.items.map((r) => r.id))
    cursor = page.nextCursor ?? ''
  } while (cursor)
  for (const [i, id] of ids.entries()) {
    await api.put<ImportRow>(`/admin/imports/${batchId}/rows/${id}`, { decision: 'skip' })
    onProgress(i + 1, ids.length)
  }
}

export function useSkipAllPending(batchId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (onProgress: (done: number, total: number) => void) => skipAllPending(batchId, onProgress),
    // Even a partial run changed rows, so reload the counts and the rows.
    onSettled: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: ['admin', 'import', batchId] }),
        queryClient.invalidateQueries({ queryKey: ['admin', 'import-rows', batchId] }),
      ]),
  })
}

/** Commits (applies) or aborts (discards) a batch. */
export function useFinishBatch(batchId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    // The batch is refetched afterwards, so the response body is not needed.
    mutationFn: async ({ commit, retailPricesPublic = false }: { commit: boolean; retailPricesPublic?: boolean }) => {
      if (commit) await api.post<ImportBatch>(`/admin/imports/${batchId}/commit`, { retailPricesPublic })
      else await api.post<void>(`/admin/imports/${batchId}/abort`)
    },
    onSuccess: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: ['admin', 'import', batchId] }),
        queryClient.invalidateQueries({ queryKey: ['admin', 'imports'] }),
        queryClient.invalidateQueries({ queryKey: ['admin', 'import-rows', batchId] }),
        // A commit creates and updates products.
        queryClient.invalidateQueries({ queryKey: ['admin', 'products'] }),
        queryClient.invalidateQueries({ queryKey: ['admin', 'product-count'] }),
      ]),
  })
}
