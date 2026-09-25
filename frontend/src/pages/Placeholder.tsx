/** Stands in for a page that a later step builds. */
export function Placeholder({ title }: { title: string }) {
  return (
    <section>
      <h1>{title}</h1>
      <p className="muted">This page is coming soon.</p>
    </section>
  )
}
