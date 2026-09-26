// Who is signed in, as one cached query. A 401 from GET /auth/session means
// "nobody", not an error. Signing in fills the cache directly; signing out
// (or any admin request answering 401, see api/queryClient.ts) empties it,
// and the admin guard reacts to that by showing the sign-in page.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError, setCsrfToken } from '../api/client'
import type { Session } from '../api/types'
import { forgetDeviceToken } from './deviceToken'

export const sessionKey = ['session'] as const

async function fetchSession(): Promise<Session | null> {
  try {
    const session = await api.get<Session>('/auth/session')
    setCsrfToken(session.csrfToken)
    return session
  } catch (err) {
    if (err instanceof ApiError && err.status === 401) {
      setCsrfToken(undefined)
      return null
    }
    throw err
  }
}

export function useSession() {
  return useQuery({ queryKey: sessionKey, queryFn: fetchSession, staleTime: Infinity, retry: false })
}

export function useLogin() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: { email: string; password: string }) => api.post<Session>('/auth/login', input),
    onSuccess: (session) => {
      setCsrfToken(session.csrfToken)
      queryClient.setQueryData(sessionKey, session)
    },
  })
}

/**
 * Signs out. A browser session is ended on the server; a paired phone can
 * only be revoked from the browser admin, so here it just forgets its token.
 * Either way every cached admin answer is dropped.
 */
export function useLogout() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (session: Session) => {
      if (session.kind === 'device') forgetDeviceToken()
      else await api.post<void>('/auth/logout')
    },
    onSettled: () => {
      setCsrfToken(undefined)
      queryClient.removeQueries({ predicate: (q) => q.queryKey[0] === 'admin' })
      queryClient.setQueryData(sessionKey, null)
    },
  })
}
