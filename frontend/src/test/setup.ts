import { cleanup } from '@testing-library/react'
import { afterEach, vi } from 'vitest'
// Adds DOM matchers such as toBeInTheDocument() to Vitest's expect.
import '@testing-library/jest-dom/vitest'

// jsdom does not implement scrolling; the router's scroll restoration calls it.
window.scrollTo = () => {}

afterEach(() => {
  cleanup() // unmount what the test rendered, so tests cannot see each other's pages
  vi.unstubAllGlobals()
})
