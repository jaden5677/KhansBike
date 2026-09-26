import { useState, type FormEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import { useAdminCategories, type Versioned } from '../api/admin'
import type { AdminCategory, Binding } from '../api/adminTypes'
import {
  useAdminAttributes,
  useAdminCategory,
  useBindAttribute,
  useBindings,
  useDeleteCategory,
  useSaveCategory,
  useUnbindAttribute,
} from '../api/adminTaxonomy'
import { ApiError } from '../api/client'
import { ErrorText, fieldError } from '../components/ErrorText'
import { Loadable } from '../components/Loadable'
import styles from './admin.module.css'
import { ParentSelect } from './CategoriesPage'

export function CategoryEditPage() {
  const { id = '' } = useParams()
  const category = useAdminCategory(id)
  return (
    <>
      <p>
        <Link to="/admin/categories">← Categories</Link>
      </p>
      <Loadable query={category}>
        {(loaded) => (
          <>
            {/* Keyed by id: after a save the refetched ETag arrives as a new
                prop, and the form keeps its "Saved." message. */}
            <CategoryForm key={loaded.data.id} loaded={loaded} />
            <Bindings categoryId={id} />
          </>
        )}
      </Loadable>
    </>
  )
}

function CategoryForm({ loaded }: { loaded: Versioned<AdminCategory> }) {
  const c = loaded.data
  const navigate = useNavigate()
  const categories = useAdminCategories()
  const save = useSaveCategory()
  const remove = useDeleteCategory()
  const [name, setName] = useState(c.name)
  const [slug, setSlug] = useState(c.slug)
  const [parentId, setParentId] = useState(c.parentId ?? '')
  const [description, setDescription] = useState(c.description ?? '')
  const [isActive, setIsActive] = useState(c.isActive)
  const [confirmDelete, setConfirmDelete] = useState(false)

  function submit(e: FormEvent) {
    e.preventDefault()
    save.mutate({
      id: c.id,
      etag: loaded.etag,
      input: {
        name: name.trim(),
        slug: slug.trim(),
        parentId: parentId || null,
        position: c.position,
        description: description.trim() || null,
        isActive,
      },
    })
  }

  const conflict = save.error instanceof ApiError && save.error.status === 412
  return (
    <>
      <title>{`${c.name} | Categories | Khan's Bike Zone admin`}</title>
      <h1>{c.name}</h1>
      <form onSubmit={submit} className={`stack ${styles.section}`}>
        <div className="field">
          <label>
            Name
            <input value={name} onChange={(e) => setName(e.target.value)} />
          </label>
          {fieldError(save.error, 'name') && <span className="error-text">{fieldError(save.error, 'name')}</span>}
        </div>
        <div className="field">
          <label>
            Web address
            <input value={slug} onChange={(e) => setSlug(e.target.value)} />
          </label>
          <span className="muted">/c/{slug}</span>
          {fieldError(save.error, 'slug') && <span className="error-text">{fieldError(save.error, 'slug')}</span>}
        </div>
        <label>
          Inside (moving a category moves everything under it)
          <ParentSelect categories={categories.data ?? []} value={parentId} onChange={setParentId} exclude={c} />
        </label>
        <div className="field field-wide">
          <label>
            Description
            <textarea rows={3} value={description} onChange={(e) => setDescription(e.target.value)} />
          </label>
        </div>
        <div className="checkbox-list">
          <label>
            <input type="checkbox" checked={isActive} onChange={(e) => setIsActive(e.target.checked)} />
            Visible on the site (hiding it also hides everything under it)
          </label>
        </div>
        {conflict ? (
          <p role="alert" className="notice">
            Someone else changed this category. Reload the page to see their version.
          </p>
        ) : (
          !fieldError(save.error, 'name') && !fieldError(save.error, 'slug') && <ErrorText error={save.error} />
        )}
        {save.isSuccess && <p role="status">Saved.</p>}
        <div className={styles.inline}>
          <button type="submit" className="primary" disabled={save.isPending}>
            Save category
          </button>
          {confirmDelete ? (
            <>
              <span>Delete “{c.name}”?</span>
              <button
                type="button"
                onClick={() => remove.mutate(c.id, { onSuccess: () => navigate('/admin/categories', { replace: true }) })}
              >
                Yes, delete
              </button>
              <button type="button" onClick={() => setConfirmDelete(false)}>
                Keep it
              </button>
            </>
          ) : (
            <button type="button" className="link-button" onClick={() => setConfirmDelete(true)}>
              Delete category
            </button>
          )}
        </div>
        {/* e.g. 409: it still has products or subcategories */}
        <ErrorText error={remove.error} />
      </form>
    </>
  )
}

/** The attributes this category's products have, and how each is used. */
function Bindings({ categoryId }: { categoryId: string }) {
  const bindings = useBindings(categoryId)
  const attributes = useAdminAttributes()
  const bind = useBindAttribute(categoryId)
  const unbind = useUnbindAttribute(categoryId)
  const [adding, setAdding] = useState('')
  const [confirmRemove, setConfirmRemove] = useState<string | null>(null)

  function change(b: Binding, patch: Partial<Pick<Binding, 'isRequired' | 'isVariantAxis'>>) {
    bind.mutate({
      attributeId: b.attribute.id,
      input: {
        position: b.position,
        isRequired: patch.isRequired ?? b.isRequired,
        isVariantAxis: patch.isVariantAxis ?? b.isVariantAxis,
        labelOverride: b.labelOverride,
      },
    })
  }

  return (
    <section className={styles.section}>
      <h2>Product details in this category</h2>
      <p className="muted">
        These become the fields of the product form and the filters customers see. “Varies per variant” puts the field
        on each variant (like colour) instead of on the product.
      </p>
      <Loadable query={bindings}>
        {(list) => {
          const unbound = (attributes.data ?? []).filter((a) => !list.some((b) => b.attribute.id === a.id))
          return (
            <>
              {list.length === 0 ? (
                <p className="muted">None yet.</p>
              ) : (
                <table className={styles.table}>
                  <thead>
                    <tr>
                      <th>Attribute</th>
                      <th>Needed to publish</th>
                      <th>Varies per variant</th>
                      <th />
                    </tr>
                  </thead>
                  <tbody>
                    {list.map((b) => (
                      <tr key={b.attribute.id}>
                        <td>
                          <Link to={`/admin/attributes/${b.attribute.id}`}>{b.effectiveLabel}</Link>
                        </td>
                        <td>
                          <input
                            type="checkbox"
                            aria-label={`${b.effectiveLabel} needed to publish`}
                            checked={b.isRequired}
                            onChange={(e) => change(b, { isRequired: e.target.checked })}
                          />
                        </td>
                        <td>
                          <input
                            type="checkbox"
                            aria-label={`${b.effectiveLabel} varies per variant`}
                            checked={b.isVariantAxis}
                            onChange={(e) => change(b, { isVariantAxis: e.target.checked })}
                          />
                        </td>
                        <td>
                          {confirmRemove === b.attribute.id ? (
                            <span className={styles.inline}>
                              Removes its values from every product here.
                              <button
                                type="button"
                                onClick={() => unbind.mutate(b.attribute.id, { onSuccess: () => setConfirmRemove(null) })}
                              >
                                Remove
                              </button>
                              <button type="button" onClick={() => setConfirmRemove(null)}>
                                Cancel
                              </button>
                            </span>
                          ) : (
                            <button type="button" className="link-button" onClick={() => setConfirmRemove(b.attribute.id)}>
                              Remove
                            </button>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
              <div className={styles.inline}>
                <label>
                  Attribute to add{' '}
                  <select value={adding} onChange={(e) => setAdding(e.target.value)}>
                    <option value="">Choose an attribute…</option>
                    {unbound.map((a) => (
                      <option key={a.id} value={a.id}>
                        {a.label}
                      </option>
                    ))}
                  </select>
                </label>
                <button
                  type="button"
                  disabled={!adding || bind.isPending}
                  onClick={() =>
                    bind.mutate(
                      {
                        attributeId: adding,
                        input: { position: list.length, isRequired: false, isVariantAxis: false, labelOverride: null },
                      },
                      { onSuccess: () => setAdding('') },
                    )
                  }
                >
                  Add attribute
                </button>
                <Link to="/admin/attributes">Manage attributes</Link>
              </div>
              {/* e.g. 409: can't change "varies per variant" while products have values */}
              <ErrorText error={bind.error ?? unbind.error} />
            </>
          )
        }}
      </Loadable>
    </section>
  )
}
