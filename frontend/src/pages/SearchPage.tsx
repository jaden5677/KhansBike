import { useSearchParams } from 'react-router'
import { useSearch } from '../api/catalog'
import { Loadable } from '../components/Loadable'
import { SearchGroups } from '../components/SearchGroups'

export function SearchPage() {
  const [params] = useSearchParams()
  const q = (params.get('q') ?? '').trim()
  const results = useSearch(q)

  if (!q) {
    return (
      <>
        <title>Search | Khan's Bike Zone</title>
        <h1>Search</h1>
        <p className="muted">Type what you're looking for in the search box above.</p>
      </>
    )
  }
  return (
    <>
      <title>{`${q} | Search | Khan's Bike Zone`}</title>
      <h1>Results for “{q}”</h1>
      <Loadable query={results}>
        {(groups) =>
          groups.length === 0 ? (
            <p>Nothing matched. Try a different word, or a part or model number.</p>
          ) : (
            // "See all" opens the category listing with the same search text,
            // where it can be filtered further.
            <SearchGroups groups={groups} seeAll={(g) => `/c/${g.category.slug}?q=${encodeURIComponent(q)}`} />
          )
        }
      </Loadable>
    </>
  )
}
