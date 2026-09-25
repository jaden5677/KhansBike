import { useMemo, useState, type CSSProperties } from 'react'
import type { Image, ImageSource } from '../api/types'
import { blurhashToDataURL } from '../lib/blurhash'

interface Props {
  image: Image
  alt: string
  /**
   * How wide the image is drawn, so the browser can pick the smallest file
   * that is still sharp, e.g. "(min-width: 60rem) 25vw, 50vw".
   */
  sizes: string
  className?: string
  /** Load immediately (the main product photo) instead of when scrolled near. */
  eager?: boolean
}

function srcSet(sources: ImageSource[]): string {
  return sources.map((s) => `${s.url} ${s.width}w`).join(', ')
}

/**
 * A responsive image from the API's renditions: WebP for browsers that take
 * it, JPEG otherwise. width/height reserve the right space before the file
 * arrives (no layout jump), and the dominant colour plus blurhash fill that
 * space until it does.
 */
export function Picture({ image, alt, sizes, className, eager = false }: Props) {
  const [loaded, setLoaded] = useState(false)
  const placeholder = useMemo(() => (image.blurhash ? blurhashToDataURL(image.blurhash) : null), [image.blurhash])

  const bySize = [...image.sources].sort((a, b) => a.width - b.width)
  const webp = bySize.filter((s) => s.format === 'webp')
  const jpeg = bySize.filter((s) => s.format === 'jpeg')
  const fallback = jpeg.at(-1) ?? bySize.at(-1)

  // Removed once loaded, so images with transparent areas don't show it.
  const style: CSSProperties = loaded
    ? {}
    : {
        backgroundColor: image.dominantHex,
        backgroundImage: placeholder ? `url(${placeholder})` : undefined,
        backgroundSize: 'cover',
      }

  return (
    <picture>
      {webp.length > 0 && <source type="image/webp" srcSet={srcSet(webp)} sizes={sizes} />}
      <img
        className={className}
        src={fallback?.url}
        srcSet={jpeg.length > 0 ? srcSet(jpeg) : undefined}
        sizes={sizes}
        width={image.width}
        height={image.height}
        alt={image.altText ?? alt}
        loading={eager ? 'eager' : 'lazy'}
        decoding="async"
        style={style}
        onLoad={() => setLoaded(true)}
      />
    </picture>
  )
}
