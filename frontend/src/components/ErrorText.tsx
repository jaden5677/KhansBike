import { ApiError } from '../api/client'

/** A readable message for a failed request, written for the visitor. */
export function errorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 429) return 'Too many attempts. Please wait a minute and try again.'
    if (error.status === 0 || error.status >= 500) return 'Something went wrong on our side. Please try again.'
    return error.message
  }
  return 'Something went wrong. Please try again.'
}

/** The error under a form. role="alert" makes screen readers announce it. */
export function ErrorText({ error }: { error: unknown }) {
  if (!error) return null
  return (
    <p role="alert" className="error-text">
      {errorMessage(error)}
    </p>
  )
}

/** The server's message for one field, e.g. "email: must be a valid address". */
export function fieldError(error: unknown, field: string): string | undefined {
  return error instanceof ApiError ? error.fieldMessage(field) : undefined
}
