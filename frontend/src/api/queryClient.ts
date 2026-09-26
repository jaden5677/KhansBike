import { MutationCache, QueryCache, QueryClient } from '@tanstack/react-query'
import { ApiError } from './client'

/**
 * Any request answering 401 means the sign-in has ended (session expired,
 * phone revoked). Recording "nobody is signed in" in the session query makes
 * the admin guard show the sign-in page, so no screen needs its own check.
 */
function signOutOn401(client: () => QueryClient) {
  return (error: Error) => {
    if (error instanceof ApiError && error.status === 401) client().setQueryData(['session'], null)
  }
}

/**
 * Builds the shared cache for server data. The public catalogue is cacheable
 * for 60 seconds on the server, so the client treats data as fresh for as
 * long. Client errors (4xx) are final answers and are not retried; network
 * and server errors get one more try.
 */
export function createQueryClient(options: { retry?: boolean } = {}): QueryClient {
  const client: QueryClient = new QueryClient({
    queryCache: new QueryCache({ onError: signOutOn401(() => client) }),
    mutationCache: new MutationCache({ onError: signOutOn401(() => client) }),
    defaultOptions: {
      queries: {
        staleTime: 60_000,
        retry:
          options.retry === false
            ? false
            : (failures, error) => {
                if (error instanceof ApiError && error.status >= 400 && error.status < 500) return false
                return failures < 1
              },
      },
    },
  })
  return client
}

export const queryClient = createQueryClient()
