import { Link } from 'react-router'

export function NotFound() {
  return (
    <section>
      <h1>Page not found</h1>
      <p>
        That page doesn't exist. <Link to="/">Back to the shop</Link>
      </p>
    </section>
  )
}
