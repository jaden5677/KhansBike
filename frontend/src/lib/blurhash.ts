import { decode } from 'blurhash'

const cache = new Map<string, string | null>()

/**
 * Turns a blurhash (a ~30-character description of a blurry version of an
 * image) into a tiny data: URL to show while the real image downloads.
 * Results are cached; null when the browser cannot draw it.
 */
export function blurhashToDataURL(hash: string): string | null {
  const hit = cache.get(hash)
  if (hit !== undefined) return hit
  let url: string | null = null
  try {
    const size = 32
    const pixels = decode(hash, size, size)
    const canvas = document.createElement('canvas')
    canvas.width = size
    canvas.height = size
    const ctx = canvas.getContext('2d')
    if (ctx) {
      const data = ctx.createImageData(size, size)
      data.data.set(pixels)
      ctx.putImageData(data, 0, 0)
      url = canvas.toDataURL()
    }
  } catch {
    url = null // an invalid hash just means no placeholder
  }
  cache.set(hash, url)
  return url
}
