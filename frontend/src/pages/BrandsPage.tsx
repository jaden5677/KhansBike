import { Link, useParams } from 'react-router'
import { useBrands } from '../api/catalog'
import { Loadable } from '../components/Loadable'
import { Picture } from '../components/Picture'
import { ProductList } from '../components/ProductList'
import { NotFound } from './NotFound'

export function BrandsPage() {
  const brands = useBrands()
  return (
    <>
      <title>Brands | Khan's Bike Zone</title>
      <h1>Brands</h1>
      <Loadable query={brands}>
        {(list) => (
          <ul className="tiles">
            {list.map((b) => (
              <li key={b.id}>
                <Link to={`/brands/${b.slug}`} className="tile">
                  {b.logo && <Picture image={b.logo} alt="" sizes="10rem" />}
                  <span>{b.name}</span>
                </Link>
              </li>
            ))}
          </ul>
        )}
      </Loadable>
    </>
  )
}

/** One brand's products. The name comes from the (cached) brand list. */
export function BrandPage() {
  const { slug = '' } = useParams()
  const brands = useBrands()
  return (
    <Loadable query={brands}>
      {(list) => {
        const brand = list.find((b) => b.slug === slug)
        if (!brand) return <NotFound />
        return (
          <>
            <title>{`${brand.name} | Khan's Bike Zone`}</title>
            <h1>{brand.name}</h1>
            <ProductList query={new URLSearchParams({ brand: slug })} />
          </>
        )
      }}
    </Loadable>
  )
}
