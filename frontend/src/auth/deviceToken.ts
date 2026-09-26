import { setDeviceToken } from '../api/client'

// A paired phone signs in with a long-lived Bearer token instead of a
// session cookie. It is kept in localStorage so the phone stays signed in
// across restarts. That is safe for a Bearer token in a way it would not be
// for a cookie: the browser never attaches it to requests on its own, so
// another site cannot make the phone send it (no CSRF).

const KEY = 'bz.deviceToken'

function storage(): Storage | null {
  try {
    return window.localStorage
  } catch {
    return null // private mode or blocked storage: pairing lasts this tab only
  }
}

/** Applies a stored token, if any. Call once at startup. */
export function restoreDeviceToken(): boolean {
  const token = storage()?.getItem(KEY) ?? undefined
  setDeviceToken(token)
  return token !== undefined
}

export function saveDeviceToken(token: string): void {
  try {
    storage()?.setItem(KEY, token)
  } catch {
    // Storage full or blocked: the token still works until the tab closes.
  }
  setDeviceToken(token)
}

/** Forgets this phone's pairing locally (revoking it happens in the admin). */
export function forgetDeviceToken(): void {
  try {
    storage()?.removeItem(KEY)
  } catch {
    // Nothing stored, nothing to remove.
  }
  setDeviceToken(undefined)
}

export function hasDeviceToken(): boolean {
  return (storage()?.getItem(KEY) ?? null) !== null
}
