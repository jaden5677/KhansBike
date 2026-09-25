import { useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router'
import { useSuggestions } from '../api/catalog'
import { useDebounced } from '../lib/useDebounced'
import styles from './SearchBox.module.css'

/**
 * The header search: submitting opens the results page, and while typing a
 * short list of matching product names appears (asked for once the person
 * pauses for 200 ms, not on every keystroke).
 */
export function SearchBox() {
  const navigate = useNavigate()
  const [text, setText] = useState('')
  const [open, setOpen] = useState(false)
  const suggestions = useSuggestions(useDebounced(text, 200))
  const items = text.trim().length >= 2 ? (suggestions.data ?? []) : []

  function submit(e: FormEvent) {
    e.preventDefault()
    const q = text.trim()
    if (!q) return
    setOpen(false)
    navigate(`/search?q=${encodeURIComponent(q)}`)
  }

  function close() {
    setOpen(false)
    setText('')
  }

  return (
    <form role="search" className={styles.form} onSubmit={submit}>
      <input
        type="search"
        aria-label="Search products"
        placeholder="Search bikes and parts"
        value={text}
        onChange={(e) => {
          setText(e.target.value)
          setOpen(true)
        }}
        onFocus={() => setOpen(true)}
        onBlur={() => setOpen(false)}
        onKeyDown={(e) => e.key === 'Escape' && setOpen(false)}
      />
      <button type="submit">Search</button>
      {open && items.length > 0 && (
        // preventDefault on mousedown keeps focus in the input, so the blur
        // above does not close the list before the click lands on a link.
        <ul className={styles.suggestions} aria-label="Suggestions" onMouseDown={(e) => e.preventDefault()}>
          {items.map((s) => (
            <li key={s.slug}>
              <Link to={`/p/${s.slug}`} onClick={close}>
                {s.name}
              </Link>
            </li>
          ))}
        </ul>
      )}
    </form>
  )
}
