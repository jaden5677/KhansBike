// Account security for the browser admin: paired phones and the password.
// The server only allows these from a browser session, never from a phone.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { Device, PairingCode } from '../api/adminTypes'
import { api } from '../api/client'

/** Paired phones; `refreshEvery` (ms) keeps checking, e.g. while a pairing code is shown. */
export function useDevices(refreshEvery: number | false = false) {
  return useQuery({
    queryKey: ['admin', 'devices'],
    queryFn: ({ signal }) => api.get<{ items: Device[] }>('/admin/devices', undefined, signal).then((r) => r.items),
    refetchInterval: refreshEvery,
  })
}

/** A fresh single-use pairing code (valid for 10 minutes). */
export function useCreatePairingCode() {
  return useMutation({ mutationFn: () => api.post<PairingCode>('/admin/devices/pairing-codes') })
}

export function useRevokeDevice() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => api.delete(`/admin/devices/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['admin', 'devices'] }),
  })
}

export function useChangePassword() {
  return useMutation({
    mutationFn: (input: { currentPassword: string; newPassword: string }) => api.put<void>('/auth/password', input),
  })
}
