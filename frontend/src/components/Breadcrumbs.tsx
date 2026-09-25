import { Link } from 'react-router'
import type { CategoryRef } from '../api/types'

/** Home › Wheels › Tyres, with the current page unlinked. */
export function Breadcrumbs({ trail, current }: { trail: CategoryRef[]; current: string }) {
  return (
    <nav aria-label="Breadcrumb" className="breadcrumbs">
      <ol>
        <li>
          <Link to="/">Home</Link>
        </li>
        {trail.map((c) => (
          <li key={c.id}>
            <Link to={`/c/${c.slug}`}>{c.name}</Link>
          </li>
        ))}
        <li aria-current="page">{current}</li>
      </ol>
    </nav>
  )
}
