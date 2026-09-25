import { useEffect, useState, type ChangeEvent } from 'react'
import type { AdminMedia, AdminVariant, MediaInput, MediaRole } from '../api/adminTypes'
import { attachMedia, detachMedia, updateMedia, uploadImage } from '../api/adminTaxonomy'
import { ErrorText } from '../components/ErrorText'
import { Picture } from '../components/Picture'
import styles from './admin.module.css'

const roles: { role: MediaRole; label: string }[] = [
  { role: 'hero', label: 'Main photo' },
  { role: 'gallery', label: 'Gallery' },
  { role: 'detail', label: 'Detail' },
  { role: 'swatch', label: 'Colour swatch' },
]

interface Props {
  productId: string
  media: AdminMedia[]
  variants: AdminVariant[]
  /**
   * Re-reads the product after a photo change. Photo changes bump the
   * product's version, so the editor must take the new ETag, or its next
   * save would be refused as a conflict.
   */
  refresh: () => Promise<void>
}

const toInput = (m: AdminMedia, patch: Partial<MediaInput> = {}): MediaInput => ({
  assetId: m.assetId,
  variantId: m.variantId,
  role: m.role,
  position: m.position,
  altText: m.altText,
  ...patch,
})

/**
 * The product's photos. Each change is saved straight away (the API keeps
 * photos separate from the product document), and uploads are processed in
 * the background, so the list checks back while any photo is unfinished.
 */
export function ProductPhotos({ productId, media, variants, refresh }: Props) {
  const [busy, setBusy] = useState<string | null>(null)
  const [error, setError] = useState<unknown>(null)
  const processing = media.some((m) => m.status === 'pending' || m.status === 'processing')

  useEffect(() => {
    if (!processing) return
    const timer = setInterval(() => void refresh().catch(() => {}), 2000)
    return () => clearInterval(timer)
  }, [processing, refresh])

  async function run(label: string, action: () => Promise<unknown>) {
    setBusy(label)
    setError(null)
    try {
      await action()
      await refresh()
    } catch (err) {
      setError(err)
    } finally {
      setBusy(null)
    }
  }

  function upload(e: ChangeEvent<HTMLInputElement>) {
    const files = [...(e.target.files ?? [])]
    e.target.value = '' // choosing the same file again should upload again
    void run('Uploading…', async () => {
      let position = media.length
      for (const file of files) {
        const asset = await uploadImage(file)
        // The first photo of a product becomes its main photo.
        const role: MediaRole = position === 0 ? 'hero' : 'gallery'
        await attachMedia(productId, { assetId: asset.id, variantId: null, role, position, altText: null })
        position++
      }
    })
  }

  return (
    <fieldset className={styles.section}>
      <legend>Photos</legend>
      <p className="muted">JPEG, PNG or WebP. Photos are saved as soon as you add or change them.</p>
      {media.length > 0 && (
        <ul className={styles.photos}>
          {[...media]
            .sort((a, b) => a.position - b.position)
            .map((m) => (
              <li key={m.id} className={styles.photo}>
                <div className={styles.photoImage}>
                  {m.status === 'ready' && m.image ? (
                    <Picture image={m.image} alt={m.altText ?? ''} sizes="10rem" />
                  ) : (
                    <span className="muted">{m.status === 'failed' ? 'Could not process this image' : 'Processing…'}</span>
                  )}
                </div>
                <label>
                  Use as
                  <select
                    value={m.role}
                    onChange={(e) =>
                      void run('Saving…', () => updateMedia(productId, m.id, toInput(m, { role: e.target.value as MediaRole })))
                    }
                  >
                    {roles.map((r) => (
                      <option key={r.role} value={r.role}>
                        {r.label}
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  Shows
                  <select
                    value={m.variantId ?? ''}
                    onChange={(e) =>
                      void run('Saving…', () =>
                        updateMedia(productId, m.id, toInput(m, { variantId: e.target.value || null })),
                      )
                    }
                  >
                    <option value="">Every variant</option>
                    {variants.map((v) => (
                      <option key={v.id} value={v.id}>
                        {v.nameSuffix ?? v.sku}
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  Description for screen readers
                  <input
                    defaultValue={m.altText ?? ''}
                    placeholder="e.g. Black grips on a handlebar"
                    // Saved when you leave the field, not on every keystroke.
                    onBlur={(e) => {
                      const altText = e.target.value.trim() || null
                      if (altText !== m.altText) void run('Saving…', () => updateMedia(productId, m.id, toInput(m, { altText })))
                    }}
                  />
                </label>
                <button type="button" className="link-button" onClick={() => void run('Removing…', () => detachMedia(productId, m.id))}>
                  Remove photo
                </button>
              </li>
            ))}
        </ul>
      )}
      <label className={styles.inline}>
        Add photos
        <input type="file" accept="image/jpeg,image/png,image/webp" multiple onChange={upload} disabled={busy !== null} />
      </label>
      {busy && (
        <p role="status" className="muted">
          {busy}
        </p>
      )}
      <ErrorText error={error} />
    </fieldset>
  )
}
