import { useEffect, useState } from 'react'

/**
 * Returns value once it has stopped changing for `delay` ms. Used by the
 * search box so we ask for suggestions when the person pauses, not on every
 * keystroke.
 */
export function useDebounced<T>(value: T, delay: number): T {
  const [settled, setSettled] = useState(value)
  useEffect(() => {
    const timer = setTimeout(() => setSettled(value), delay)
    return () => clearTimeout(timer)
  }, [value, delay])
  return settled
}
