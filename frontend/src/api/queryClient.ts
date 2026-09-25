import { QueryClient } from '@tanstack/react-query'
import { ApiError } from './client'

/**
 * Shared cache for server data. The public catalogue is cacheable for 60
 * seconds on the server, so the client treats data as fresh for as long.
 * Client errors (4xx) are final answers and are not retried; network and
 * server errors get one more try.
 */
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 60_000,
      retry: (failures, error) => {
        if (error instanceof ApiError && error.status >= 400 && error.status < 500) return false
        return failures < 1
      },
    },
  },
})
