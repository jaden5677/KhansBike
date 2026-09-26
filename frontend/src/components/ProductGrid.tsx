import { Link } from 'react-router'
import type { ProductCard as Card } from '../api/types'
import { Picture } from './Picture'
import { Price } from './Price'
import styles from './ProductGrid.module.css'
import { StockBadge } from './StockBadge'

export function ProductCard({ product }: { product: Card }) {
  return (
    <Link to={`/p/${product.slug}`} className={styles.card}>
      <div className={styles.media}>
        {product.image ? (
          <Picture image={product.image} alt={product.name} sizes="(min-width: 60rem) 18rem, 50vw" />
        ) : (
          <div className={styles.noImage} aria-hidden="true" />
        )}
      </div>
      <div className={styles.body}>
        {product.brand && <span className="muted">{product.brand.name}</span>}
        <span className={styles.name}>{product.name}</span>
        <Price money={product.fromPrice} from={product.variantCount > 1} />
        <StockBadge status={product.availability} />
      </div>
    </Link>
  )
}

export function ProductGrid({ products }: { products: Card[] }) {
  return (
    <ul className={styles.grid}>
      {products.map((p) => (
        <li key={p.id}>
          <ProductCard product={p} />
        </li>
      ))}
    </ul>
  )
}
