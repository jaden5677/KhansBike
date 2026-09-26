import { useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router'
import { useProduct } from '../api/catalog'
import type { AttributeValue, ProductDetail, Variant } from '../api/types'
import { Breadcrumbs } from '../components/Breadcrumbs'
import { Loadable } from '../components/Loadable'
import { Picture } from '../components/Picture'
import { Price } from '../components/Price'
import { StockBadge } from '../components/StockBadge'
import styles from './ProductPage.module.css'

export function ProductPage() {
  const { slug = '' } = useParams()
  const product = useProduct(slug)
  return <Loadable query={product}>{(p) => <ProductView product={p} />}</Loadable>
}

/** How a variant is named in the picker: its suffix, or what makes it different. */
function variantLabel(v: Variant): string {
  return v.nameSuffix ?? (v.attributes.map((a) => a.display).join(' / ') || v.sku)
}

function ProductView({ product }: { product: ProductDetail }) {
  // The chosen variant lives in the URL (?variant=), so a link can point
  // straight at the red one.
  const [params, setParams] = useSearchParams()
  const variant =
    product.variants.find((v) => v.id === params.get('variant')) ??
    product.variants.find((v) => v.isDefault) ??
    product.variants[0]

  function choose(v: Variant) {
    const next = new URLSearchParams(params)
    next.set('variant', v.id)
    setParams(next, { replace: true, preventScrollReset: true })
  }

  const specs: AttributeValue[] = [...product.attributes, ...(variant?.attributes ?? [])]
  const wheelSize = specs.find((a) => a.key === 'wheel_size' && typeof a.value === 'string')

  return (
    <article>
      <title>{`${product.name} | Khan's Bike Zone`}</title>
      <Breadcrumbs trail={[product.category]} current={product.name} />
      <div className={styles.layout}>
        {/* key: a different variant starts again at its first photo. */}
        <Gallery key={variant?.id} product={product} variantId={variant?.id} />

        <div className={styles.info}>
          {product.brand && <p className="muted">{product.brand.name}</p>}
          <h1>
            {product.name}
            {variant?.nameSuffix && <span className="muted"> {variant.nameSuffix}</span>}
          </h1>
          {variant && (
            <>
              <p className={styles.price}>
                <Price money={variant.price} />
              </p>
              <p>
                <StockBadge status={variant.stockStatus} />
              </p>
            </>
          )}

          {product.variants.length > 1 && (
            <div role="group" aria-label="Options" className={styles.variants}>
              {product.variants.map((v) => (
                <button key={v.id} type="button" aria-pressed={v.id === variant?.id} onClick={() => choose(v)}>
                  {variantLabel(v)}
                </button>
              ))}
            </div>
          )}

          {product.summary && <p>{product.summary}</p>}
          {variant && (
            <p className="muted">
              Item code {variant.sku}
              {variant.modelNo && ` · Model ${variant.modelNo}`}
            </p>
          )}
          <p className="muted">Visit or call the shop to buy or reserve this item.</p>
        </div>
      </div>

      {/* Plain text only: descriptions are never rendered as HTML. */}
      {product.description && <p className={styles.description}>{product.description}</p>}

      {specs.length > 0 && (
        <section>
          <h2>Specifications</h2>
          <dl className={styles.specs}>
            {specs.map((a) => (
              <div key={a.key}>
                <dt>{a.label}</dt>
                <dd>
                  {a.swatchHex && <span className="swatch" style={{ background: a.swatchHex }} aria-hidden="true" />}
                  {a.display}
                </dd>
              </div>
            ))}
          </dl>
        </section>
      )}

      {wheelSize && (
        <p>
          <Link to={`/fitment/${encodeURIComponent(wheelSize.value as string)}`}>
            More parts that fit {wheelSize.display}
          </Link>
        </p>
      )}
    </article>
  )
}

/** The main photo plus thumbnails. Photos of the chosen variant come first. */
function Gallery({ product, variantId }: { product: ProductDetail; variantId?: string }) {
  const images = [
    ...product.images.filter((i) => i.variantId === variantId && variantId !== undefined),
    ...product.images.filter((i) => !i.variantId),
  ]
  const [selected, setSelected] = useState(0)
  const current = images[Math.min(selected, images.length - 1)]

  if (!current) return null // no photos yet: the details take the full width
  return (
    <div className={styles.gallery}>
      <Picture
        image={current}
        alt={product.name}
        sizes="(min-width: 60rem) 36rem, 100vw"
        className={styles.mainImage}
        eager
      />
      {images.length > 1 && (
        <ul className={styles.thumbs}>
          {images.map((img, i) => (
            <li key={img.id}>
              <button
                type="button"
                aria-label={`Photo ${i + 1} of ${images.length}`}
                aria-pressed={img === current}
                onClick={() => setSelected(i)}
              >
                <Picture image={img} alt="" sizes="4rem" />
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
