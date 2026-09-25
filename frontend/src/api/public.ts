// Customer-facing writes: the mailing list and phone pairing. These change
// something on the server, so they are TanStack Query mutations (run on
// demand, never cached or retried) rather than queries.

import { useMutation } from '@tanstack/react-query'
import { api } from './client'

export interface SubscribeInput {
  email: string
  name?: string
  /** Where the form was, so the owner can see which signups work. */
  source?: string
}

export function useSubscribe() {
  return useMutation({
    mutationFn: (input: SubscribeInput) => api.post<{ status: string }>('/subscribers', input),
  })
}

export function useConfirmSubscription() {
  return useMutation({
    mutationFn: (token: string) => api.post<void>('/subscribers/confirm', { token }),
  })
}

export function useUnsubscribe() {
  return useMutation({
    mutationFn: (token: string) => api.post<void>('/subscribers/unsubscribe', { token }),
  })
}

export interface PairedDevice {
  token: string
  device: { id: string; name: string; createdAt: string }
}

export function usePairDevice() {
  return useMutation({
    mutationFn: (input: { code: string; name: string }) => api.post<PairedDevice>('/auth/devices', input),
  })
}
