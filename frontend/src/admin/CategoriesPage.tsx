import { useState, type FormEvent } from 'react'
import { Link } from 'react-router'
import { useAdminCategories } from '../api/admin'
import type { AdminCategory } from '../api/adminTypes'
import { useSaveCategory } from '../api/adminTaxonomy'
import { ErrorText, fieldError } from '../components/ErrorText'
import { Loadable } from '../components/Loadable'
import styles from './admin.module.css'

/** Categories in tree order (sorting by path puts children after their parent). */
export function treeOrder(categories: AdminCategory[]): AdminCategory[] {
  return [...categories].sort((a, b) => a.path.localeCompare(b.path))
}

export const depth = (c: AdminCategory) => c.path.split('.').length - 1

/** A parent picker; `exclude` leaves out a category and everything under it. */
export function ParentSelect({
  categories,
  value,
  onChange,
  exclude,
}: {
  categories: AdminCategory[]
  value: string
  onChange: (id: string) => void
  exclude?: AdminCategory
}) {
  const allowed = treeOrder(categories).filter(
    (c) => !exclude || (c.id !== exclude.id && !c.path.startsWith(exclude.path + '.')),
  )
  return (
    <select value={value} onChange={(e) => onChange(e.target.value)}>
      <option value="">None (top level)</option>
      {allowed.map((c) => (
        <option key={c.id} value={c.id}>
          {'  '.repeat(depth(c))}
          {c.name}
        </option>
      ))}
    </select>
  )
}

export function CategoriesPage() {
  const categories = useAdminCategories()
  const save = useSaveCategory()
  const [name, setName] = useState('')
  const [parentId, setParentId] = useState('')

  function add(e: FormEvent) {
    e.preventDefault()
    save.mutate(
      { input: { name: name.trim(), parentId: parentId || null, position: 0, description: null, isActive: true } },
      { onSuccess: () => setName('') },
    )
  }

  return (
    <>
      <title>Categories | Khan's Bike Zone admin</title>
      <h1>Categories</h1>
      <Loadable query={categories}>
        {(list) => (
          <>
            <form onSubmit={add} className={styles.toolbar} aria-label="Add a category">
              <div className="field">
                <label>
                  New category
                  <input value={name} onChange={(e) => setName(e.target.value)} placeholder="e.g. Brake pads" />
                </label>
                {fieldError(save.error, 'name') && <span className="error-text">{fieldError(save.error, 'name')}</span>}
              </div>
              <label>
                Inside
                <ParentSelect categories={list} value={parentId} onChange={setParentId} />
              </label>
              <button type="submit" disabled={save.isPending || !name.trim()}>
                Add
              </button>
            </form>
            {!fieldError(save.error, 'name') && <ErrorText error={save.error} />}
            <ul className={styles.tree}>
              {treeOrder(list).map((c) => (
                <li key={c.id} style={{ paddingLeft: `${depth(c) * 1.5}rem` }}>
                  <Link to={`/admin/categories/${c.id}`}>{c.name}</Link>
                  {!c.isActive && <span className="badge"> hidden</span>}
                </li>
              ))}
            </ul>
          </>
        )}
      </Loadable>
    </>
  )
}
