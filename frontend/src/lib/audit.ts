// Turning audit log entries into something a shop owner can read. Pure
// functions, so they are tested on their own.

const actionLabels: Record<string, string> = {
  'product.create': 'Created product',
  'product.update': 'Edited product',
  'product.delete': 'Deleted product',
  'product.variant.update': 'Updated a variant (price/stock)',
  'product.media.attach': 'Added a photo',
  'product.media.update': 'Changed a photo',
  'product.media.detach': 'Removed a photo',
  'media.upload': 'Uploaded an image',
  'category.create': 'Created category',
  'category.update': 'Edited category',
  'category.delete': 'Deleted category',
  'category.attribute.bind': 'Attached an attribute to a category',
  'category.attribute.unbind': 'Removed an attribute from a category',
  'attribute.create': 'Created attribute',
  'attribute.update': 'Edited attribute',
  'attribute.delete': 'Deleted attribute',
  'attribute.option.create': 'Added an option',
  'attribute.option.update': 'Edited an option',
  'attribute.option.delete': 'Deleted an option',
  'brand.create': 'Created brand',
  'brand.update': 'Edited brand',
  'brand.delete': 'Deleted brand',
  'supplier.create': 'Created supplier',
  'supplier.update': 'Edited supplier',
  'supplier.delete': 'Deleted supplier',
  'import.stage': 'Uploaded a price list',
  'import.commit': 'Applied a price list',
  'import.abort': 'Discarded a price list',
  'auth.login': 'Signed in',
  'device.pair': 'Paired a phone',
  'device.revoke': 'Revoked a phone',
  'user.password_change': 'Changed password',
  'user.password_reset': 'Password reset',
}

/** "product.update" → "Edited product"; unknown actions are shown as they are. */
export function describeAction(action: string): string {
  return actionLabels[action] ?? action
}

/** Where an entry's subject can be opened in the admin, if anywhere. */
export function entityLink(entityType: string, entityId?: string): string | undefined {
  if (!entityId) return undefined
  const paths: Record<string, string> = {
    product: '/admin/products/',
    category: '/admin/categories/',
    attribute: '/admin/attributes/',
    import_batch: '/admin/imports/',
  }
  return paths[entityType] ? paths[entityType] + entityId : undefined
}

export interface FieldChange {
  field: string
  before: string
  after: string
}

function show(value: unknown): string {
  if (value === undefined || value === null || value === '') return '—'
  const text = typeof value === 'string' ? value : JSON.stringify(value)
  return text.length > 120 ? `${text.slice(0, 117)}…` : text
}

const isRecord = (v: unknown): v is Record<string, unknown> => typeof v === 'object' && v !== null && !Array.isArray(v)

/**
 * The fields that differ between two snapshots, compared field by field
 * (values as JSON, so nested lists and objects compare by content). A
 * creation has no "before" and a deletion no "after"; both still list
 * every field.
 */
export function changedFields(before: unknown, after: unknown): FieldChange[] {
  if (!isRecord(before) && !isRecord(after)) {
    return JSON.stringify(before) === JSON.stringify(after) ? [] : [{ field: 'value', before: show(before), after: show(after) }]
  }
  const b = isRecord(before) ? before : {}
  const a = isRecord(after) ? after : {}
  const fields = [...new Set([...Object.keys(b), ...Object.keys(a)])].sort()
  return fields
    .filter((f) => JSON.stringify(b[f]) !== JSON.stringify(a[f]))
    .map((f) => ({ field: f, before: show(b[f]), after: show(a[f]) }))
}
