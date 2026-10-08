import { useMemo } from 'react'
import { encode } from 'uqr'

/**
 * A QR code drawn as SVG. uqr turns the text into a grid of dark and light
 * modules; each dark module becomes a 1×1 square in a single path. Always
 * black on white, even in dark mode: phone cameras need the contrast.
 */
export function QRCode({ value, label, size = 224 }: { value: string; label: string; size?: number }) {
  const { path, modules } = useMemo(() => {
    const { data } = encode(value, { ecc: 'M', border: 2 })
    const commands: string[] = []
    data.forEach((row, y) =>
      row.forEach((dark, x) => {
        if (dark) commands.push(`M${x} ${y}h1v1h-1z`)
      }),
    )
    return { path: commands.join(''), modules: data.length }
  }, [value])

  return (
    <svg
      role="img"
      aria-label={label}
      viewBox={`0 0 ${modules} ${modules}`}
      width={size}
      height={size}
      shapeRendering="crispEdges"
    >
      <rect width={modules} height={modules} fill="#fff" />
      <path d={path} fill="#000" />
    </svg>
  )
}
