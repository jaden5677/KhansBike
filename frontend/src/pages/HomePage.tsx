import { Link } from 'react-router'
import { useCategoryTree } from '../api/catalog'
import { Loadable } from '../components/Loadable'
import { Picture } from '../components/Picture'
import { ProductList } from '../components/ProductList'

const featured = new URLSearchParams({ sort: 'featured' })

function plural(n: number, word: string) {
  return `${n} ${word}${n === 1 ? '' : 's'}`
}

export function HomePage() {
  const categories = useCategoryTree()
  return (
    <>
      <title>Khan's Bike Zone | Bicycles and parts in Trinidad &amp; Tobago</title>
      <section className="hero">
        <h1>Bikes, parts and accessories</h1>
        <p>Browse what's in stock at Khan's Bike Zone, then visit or call us to buy.</p>
      </section>

      <section>
        <h2>Shop by category</h2>
        <Loadable query={categories}>
          {(roots) => (
            <ul className="tiles">
              {/* Empty categories are hidden until they have something to show. */}
              {roots
                .filter((c) => c.productCount > 0)
                .map((c) => (
                <li key={c.id}>
                  <Link to={`/c/${c.slug}`} className="tile">
                    {c.image && <Picture image={c.image} alt="" sizes="(min-width: 60rem) 14rem, 45vw" />}
                    <span>{c.name}</span>
                    <span className="muted">{plural(c.productCount, 'product')}</span>
                  </Link>
                </li>
                ))}
            </ul>
          )}
        </Loadable>
      </section>

      <section>
        <h2>Featured</h2>
        <ProductList query={featured} limit={8} paginate={false} />
      </section>
    </>
  )
}
