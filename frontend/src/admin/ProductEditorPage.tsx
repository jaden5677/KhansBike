import { useCallback, useEffect, useRef, useState, type FormEvent, type ReactNode } from 'react'
import { Link, useBeforeUnload, useBlocker, useLocation, useNavigate, useParams } from 'react-router'
import {
  useAdminBrands,
  useAdminCategories,
  useAdminProduct,
  useAdminSuppliers,
  useDeleteProduct,
  useFormSchema,
  useSaveProduct,
  type Versioned,
} from '../api/admin'
import {
  priceTiers,
  statusLabels,
  type AdminCategory,
  type AdminProduct,
  type FormSchema,
  type ProductStatus,
} from '../api/adminTypes'
import { ApiError } from '../api/client'
import type { StockStatus } from '../api/types'
import { ErrorText } from '../components/ErrorText'
import { Loadable } from '../components/Loadable'
import {
  draftFromProduct,
  emptyDraft,
  emptyVariant,
  fieldErrors,
  toInput,
  type ProductDraft,
  type VariantDraft,
} from '../lib/productDraft'
import styles from './admin.module.css'
import { AttributeField } from './AttributeField'

const stockLabels: Record<StockStatus, string> = {
  in_stock: 'In stock',
  low: 'Low stock',
  out: 'Out of stock',
  special_order: 'Special order',
  unknown: 'Unknown',
}

/** /admin/products/new and /admin/products/:id. */
export function ProductEditorPage() {
  const { id } = useParams()
  const product = useAdminProduct(id)
  // Bumped after "reload their version", so the editor starts over from
  // the freshly loaded product.
  const [generation, setGeneration] = useState(0)

  if (!id) return <ProductEditor key="new" />
  return (
    <Loadable query={product}>
      {(loaded) => (
        <ProductEditor
          key={`${id}:${generation}`}
          loaded={loaded}
          onReload={async () => {
            await product.refetch()
            setGeneration((g) => g + 1)
          }}
        />
      )}
    </Loadable>
  )
}

/** A snapshot to compare against, to know whether there are unsaved changes. */
const snapshot = (d: ProductDraft) => JSON.stringify(toInput(d))

interface EditorProps {
  loaded?: Versioned<AdminProduct>
  onReload?: () => Promise<void>
}

function ProductEditor({ loaded, onReload }: EditorProps) {
  const navigate = useNavigate()
  const location = useLocation()
  const [draft, setDraft] = useState(() => (loaded ? draftFromProduct(loaded.data) : emptyDraft()))
  const [baseline, setBaseline] = useState(() => snapshot(draft))
  const [etag, setEtag] = useState(loaded?.etag)
  const [notice, setNotice] = useState<string | null>(
    (location.state as { created?: boolean } | null)?.created ? 'Product created.' : null,
  )
  const [confirmDelete, setConfirmDelete] = useState(false)

  const schema = useFormSchema(draft.categoryId).data
  const save = useSaveProduct()
  const remove = useDeleteProduct()
  const errors = fieldErrors(save.error)
  const conflict = save.error instanceof ApiError && save.error.status === 412
  const dirty = snapshot(draft) !== baseline

  // Leaving with unsaved changes asks first: useBlocker for links inside the
  // app, beforeunload for closing the tab. The ref lets a successful save
  // turn the guard off before it navigates.
  const dirtyRef = useRef(dirty)
  useEffect(() => {
    dirtyRef.current = dirty
  }, [dirty])
  const blocker = useBlocker(
    ({ currentLocation, nextLocation }) => dirtyRef.current && currentLocation.pathname !== nextLocation.pathname,
  )
  useBeforeUnload(
    useCallback((e: BeforeUnloadEvent) => {
      if (dirtyRef.current) e.preventDefault()
    }, []),
  )

  function update(patch: Partial<ProductDraft>) {
    setDraft((d) => ({ ...d, ...patch }))
    setNotice(null)
  }

  function updateVariant(key: string, patch: Partial<VariantDraft>) {
    setDraft((d) => ({ ...d, variants: d.variants.map((v) => (v.key === key ? { ...v, ...patch } : v)) }))
    setNotice(null)
  }

  function makeDefault(key: string) {
    setDraft((d) => ({ ...d, variants: d.variants.map((v) => ({ ...v, isDefault: v.key === key })) }))
  }

  function removeVariant(key: string) {
    setDraft((d) => {
      const variants = d.variants.filter((v) => v.key !== key)
      if (variants.length > 0 && !variants.some((v) => v.isDefault)) variants[0] = { ...variants[0], isDefault: true }
      return { ...d, variants }
    })
  }

  function submit(e: FormEvent) {
    e.preventDefault()
    save.mutate(
      { id: loaded?.data.id, input: toInput(draft, schema), etag },
      {
        onSuccess: (saved) => {
          dirtyRef.current = false
          if (!loaded) {
            navigate(`/admin/products/${saved.data.id}`, { replace: true, state: { created: true } })
            return
          }
          const next = draftFromProduct(saved.data)
          setDraft(next)
          setBaseline(snapshot(next))
          setEtag(saved.etag)
          setNotice('Saved.')
        },
      },
    )
  }

  function deleteProduct() {
    if (!loaded) return
    remove.mutate(loaded.data.id, {
      onSuccess: () => {
        dirtyRef.current = false
        navigate('/admin/products', { replace: true })
      },
    })
  }

  const productFields = schema?.fields.filter((f) => !f.isVariantAxis) ?? []
  const variantFields = schema?.fields.filter((f) => f.isVariantAxis) ?? []
  const title = loaded ? loaded.data.name : 'New product'

  return (
    <>
      <title>{`${title} | Khan's Bike Zone admin`}</title>
      <p>
        <Link to="/admin/products">← Products</Link>
      </p>
      <div className={styles.titleRow}>
        <h1>{title}</h1>
        {loaded?.data.status === 'active' && (
          <a href={`/p/${loaded.data.slug}`} target="_blank" rel="noreferrer">
            View on site
          </a>
        )}
      </div>

      {blocker.state === 'blocked' && (
        <div role="alertdialog" aria-label="Unsaved changes" className="notice">
          <p>You have unsaved changes. Leave without saving them?</p>
          <div className={styles.inline}>
            <button type="button" onClick={() => blocker.proceed()}>
              Leave without saving
            </button>
            <button type="button" onClick={() => blocker.reset()}>
              Stay
            </button>
          </div>
        </div>
      )}

      {conflict && (
        <div role="alert" className="notice">
          <p>
            <strong>Someone else saved this product while you were editing.</strong> Saving now would overwrite their
            changes. Reload to see their version (your unsaved edits here will be lost).
          </p>
          <button type="button" onClick={() => onReload?.()}>
            Reload their version
          </button>
        </div>
      )}

      {Object.keys(errors).length > 0 && (
        <div role="alert" className="notice">
          <p>
            <strong>Please fix these before saving:</strong>
          </p>
          <ul>
            {Object.entries(errors).map(([field, message]) => (
              <li key={field}>
                {describeField(field, schema)} {message}
              </li>
            ))}
          </ul>
        </div>
      )}
      {!conflict && Object.keys(errors).length === 0 && <ErrorText error={save.error ?? remove.error} />}
      {notice && (
        <p role="status" className="notice">
          {notice}
        </p>
      )}

      <form onSubmit={submit} className={styles.editor} noValidate>
        <fieldset className={styles.section}>
          <legend>Basics</legend>
          <div className={styles.grid}>
            <Field label="Name" error={errors.name}>
              <input value={draft.name} onChange={(e) => update({ name: e.target.value })} required />
            </Field>
            <Field label="Category" error={errors.categoryId}>
              <CategorySelect value={draft.categoryId} onChange={(categoryId) => update({ categoryId })} />
            </Field>
            <Field label="Brand" error={errors.brandId}>
              <BrandSelect value={draft.brandId} onChange={(brandId) => update({ brandId })} />
            </Field>
            <Field label="Status" error={errors.status}>
              <select value={draft.status} onChange={(e) => update({ status: e.target.value as ProductStatus })}>
                {(Object.keys(statusLabels) as ProductStatus[]).map((s) => (
                  <option key={s} value={s}>
                    {statusLabels[s]}
                  </option>
                ))}
              </select>
            </Field>
            {loaded && (
              <Field label="Web address" hint={`/p/${draft.slug}`} error={errors.slug}>
                <input value={draft.slug} onChange={(e) => update({ slug: e.target.value })} />
              </Field>
            )}
          </div>
          <Field label="Summary" hint="One line, shown on product cards." error={errors.summary} wide>
            <input value={draft.summary} onChange={(e) => update({ summary: e.target.value })} />
          </Field>
          <Field label="Description" error={errors.description} wide>
            <textarea rows={5} value={draft.description} onChange={(e) => update({ description: e.target.value })} />
          </Field>
          <div className="checkbox-list">
            <label>
              <input type="checkbox" checked={draft.isFeatured} onChange={(e) => update({ isFeatured: e.target.checked })} />
              Featured on the home page
            </label>
            <label>
              <input
                type="checkbox"
                checked={draft.retailPriceIsPublic}
                onChange={(e) => update({ retailPriceIsPublic: e.target.checked })}
              />
              Show the retail price to customers
            </label>
          </div>
        </fieldset>

        {draft.categoryId === '' ? (
          <p className="muted">Choose a category to see its fields.</p>
        ) : (
          productFields.length > 0 && (
            <fieldset className={styles.section}>
              <legend>Details</legend>
              <div className={styles.grid}>
                {productFields.map((f) => (
                  <AttributeField
                    key={f.key}
                    field={f}
                    value={draft.attributes[f.key]}
                    error={errors[`attributes.${f.key}`]}
                    onChange={(value) => update({ attributes: { ...draft.attributes, [f.key]: value } })}
                  />
                ))}
              </div>
            </fieldset>
          )
        )}

        <fieldset className={styles.section}>
          <legend>Variants</legend>
          <p className="muted">
            One per version you stock (colour, size…). Price changes are kept in the price history from today.
          </p>
          {errors.variants && <p className="error-text">{errors.variants}</p>}
          {draft.variants.map((v, i) => (
            <VariantEditor
              key={v.key}
              index={i}
              variant={v}
              fields={variantFields}
              errors={errors}
              canRemove={draft.variants.length > 1}
              onChange={(patch) => updateVariant(v.key, patch)}
              onMakeDefault={() => makeDefault(v.key)}
              onRemove={() => removeVariant(v.key)}
            />
          ))}
          <button type="button" onClick={() => update({ variants: [...draft.variants, emptyVariant()] })}>
            Add variant
          </button>
        </fieldset>

        <div className={styles.actions}>
          <button type="submit" className="primary" disabled={save.isPending || (loaded !== undefined && !dirty)}>
            {save.isPending ? 'Saving…' : loaded ? 'Save changes' : 'Create product'}
          </button>
          {dirty && <span className="muted">Unsaved changes</span>}
          {loaded &&
            (confirmDelete ? (
              <span className={styles.inline}>
                Delete this product for good?
                <button type="button" onClick={deleteProduct} disabled={remove.isPending}>
                  Yes, delete
                </button>
                <button type="button" onClick={() => setConfirmDelete(false)}>
                  Keep it
                </button>
              </span>
            ) : (
              <button type="button" className="link-button" onClick={() => setConfirmDelete(true)}>
                Delete product
              </button>
            ))}
        </div>
      </form>
    </>
  )
}

interface VariantEditorProps {
  index: number
  variant: VariantDraft
  fields: FormSchema['fields']
  errors: Record<string, string>
  canRemove: boolean
  onChange: (patch: Partial<VariantDraft>) => void
  onMakeDefault: () => void
  onRemove: () => void
}

function VariantEditor({ index, variant, fields, errors, canRemove, onChange, onMakeDefault, onRemove }: VariantEditorProps) {
  const path = `variants[${index}]`
  const suppliers = useAdminSuppliers()
  return (
    <fieldset className={styles.variant}>
      <legend>Variant {index + 1}</legend>
      <div className={styles.grid}>
        <Field label="SKU" error={errors[`${path}.sku`]}>
          <input value={variant.sku} onChange={(e) => onChange({ sku: e.target.value })} spellCheck={false} />
        </Field>
        <Field label="Name suffix" hint="Worked out from the options when blank." error={errors[`${path}.nameSuffix`]}>
          <input value={variant.nameSuffix} onChange={(e) => onChange({ nameSuffix: e.target.value })} />
        </Field>
        <Field label="Stock" error={errors[`${path}.stockStatus`]}>
          <select value={variant.stockStatus} onChange={(e) => onChange({ stockStatus: e.target.value as StockStatus })}>
            {(Object.keys(stockLabels) as StockStatus[]).map((s) => (
              <option key={s} value={s}>
                {stockLabels[s]}
              </option>
            ))}
          </select>
        </Field>
        {fields.map((f) => (
          <AttributeField
            key={f.key}
            field={f}
            value={variant.attributes[f.key]}
            error={errors[`${path}.attributes.${f.key}`]}
            onChange={(value) => onChange({ attributes: { ...variant.attributes, [f.key]: value } })}
          />
        ))}
      </div>

      <div className={styles.grid}>
        {priceTiers.map(({ tier, label }) => (
          <Field key={tier} label={label} error={errors[`${path}.prices.${tier}`]}>
            <input
              inputMode="decimal"
              placeholder="0.00"
              value={variant.prices[tier] ?? ''}
              onChange={(e) => onChange({ prices: { ...variant.prices, [tier]: e.target.value } })}
            />
          </Field>
        ))}
      </div>

      <details>
        <summary>Supplier details</summary>
        <div className={styles.grid}>
          <Field label="Supplier" error={errors[`${path}.supplierId`]}>
            <select value={variant.supplierId} onChange={(e) => onChange({ supplierId: e.target.value })}>
              <option value="">—</option>
              {suppliers.data?.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name}
                </option>
              ))}
            </select>
          </Field>
          <Field label="Supplier item no." error={errors[`${path}.supplierItemNo`]}>
            <input value={variant.supplierItemNo} onChange={(e) => onChange({ supplierItemNo: e.target.value })} />
          </Field>
          <Field label="Model no." error={errors[`${path}.modelNo`]}>
            <input value={variant.modelNo} onChange={(e) => onChange({ modelNo: e.target.value })} />
          </Field>
        </div>
      </details>

      <div className={styles.inline}>
        <label>
          <input type="radio" name="default-variant" checked={variant.isDefault} onChange={onMakeDefault} />
          Shown first
        </label>
        {canRemove && (
          <button type="button" className="link-button" onClick={onRemove}>
            Remove variant {index + 1}
          </button>
        )}
      </div>
    </fieldset>
  )
}

/** A labelled input with an optional hint and the server's message. */
interface FieldProps {
  label: string
  hint?: string
  error?: string
  /** Use the full width (long text) instead of a grid column's. */
  wide?: boolean
  children: ReactNode
}

function Field({ label, hint, error, wide = false, children }: FieldProps) {
  return (
    <div className={wide ? 'field field-wide' : 'field'}>
      <label>
        {label}
        {children}
      </label>
      {hint && <span className="muted">{hint}</span>}
      {error && <span className="error-text">{error}</span>}
    </div>
  )
}

/** Categories in tree order, indented by depth; hidden ones are marked. */
function CategorySelect({ value, onChange }: { value: string; onChange: (id: string) => void }) {
  const categories = useAdminCategories()
  const sorted = [...(categories.data ?? [])].sort((a, b) => a.path.localeCompare(b.path))
  const depth = (c: AdminCategory) => c.path.split('.').length - 1
  return (
    <select value={value} onChange={(e) => onChange(e.target.value)} required>
      <option value="">Choose…</option>
      {sorted.map((c) => (
        <option key={c.id} value={c.id}>
          {'  '.repeat(depth(c))}
          {c.name}
          {c.isActive ? '' : ' (hidden)'}
        </option>
      ))}
    </select>
  )
}

function BrandSelect({ value, onChange }: { value: string; onChange: (id: string) => void }) {
  const brands = useAdminBrands()
  return (
    <select value={value} onChange={(e) => onChange(e.target.value)}>
      <option value="">No brand</option>
      {brands.data?.map((b) => (
        <option key={b.id} value={b.id}>
          {b.name}
        </option>
      ))}
    </select>
  )
}

/** "variants[1].prices.retail_ttd" → "Variant 2 › Retail (TTD)". */
export function describeField(path: string, schema?: FormSchema): string {
  const labels: Record<string, string> = {
    name: 'Name',
    categoryId: 'Category',
    brandId: 'Brand',
    status: 'Status',
    slug: 'Web address',
    summary: 'Summary',
    description: 'Description',
    variants: 'Variants',
    sku: 'SKU',
    nameSuffix: 'Name suffix',
    stockStatus: 'Stock',
    supplierId: 'Supplier',
    supplierItemNo: 'Supplier item no.',
    modelNo: 'Model no.',
  }
  for (const { tier, label } of priceTiers) labels[tier] = label
  for (const f of schema?.fields ?? []) labels[f.key] = f.label
  return path
    .split('.')
    .filter((part) => part !== 'attributes' && part !== 'prices')
    .map((part) => {
      const variant = part.match(/^variants\[(\d+)\]$/)
      return variant ? `Variant ${Number(variant[1]) + 1}` : (labels[part] ?? part)
    })
    .join(' › ')
}
