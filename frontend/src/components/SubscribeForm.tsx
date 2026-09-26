import { useId, useState, type FormEvent } from 'react'
import { ApiError } from '../api/client'
import { useSubscribe } from '../api/public'
import { ErrorText, fieldError } from './ErrorText'

/**
 * The mailing-list signup. The API answers every signup the same way
 * (whether or not the address is already on the list), so the form can only
 * ever say "check your email", which is what keeps the list private.
 */
export function SubscribeForm({ source }: { source: string }) {
  const [email, setEmail] = useState('')
  const [name, setName] = useState('')
  const subscribe = useSubscribe()
  // Unique per form: the page can show two (footer and confirm page).
  const id = useId()

  function submit(e: FormEvent) {
    e.preventDefault()
    subscribe.mutate({ email: email.trim(), name: name.trim() || undefined, source })
  }

  if (subscribe.isSuccess) {
    return (
      <p role="status" className="notice">
        Thanks! Check your inbox for a link to confirm your subscription.
      </p>
    )
  }

  const emailError = fieldError(subscribe.error, 'email')
  const nameError = fieldError(subscribe.error, 'name')
  // Field problems are shown beside their fields; anything else below the form.
  const otherError = subscribe.error instanceof ApiError && subscribe.error.fieldErrors.length > 0 ? null : subscribe.error

  return (
    <form className="subscribe-form" onSubmit={submit} noValidate>
      <p>
        <strong>Hear about new stock and offers.</strong> We email rarely, and you can unsubscribe anytime.
      </p>
      <div className="form-row">
        {/* Errors sit beside the label, not inside it, so they are not read
            out as part of the field's name; aria-describedby links them. */}
        <div className="field">
          <label>
            Email
            <input
              type="email"
              autoComplete="email"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              aria-invalid={emailError ? true : undefined}
              aria-describedby={emailError ? `${id}-email-error` : undefined}
            />
          </label>
          {emailError && (
            <span id={`${id}-email-error`} className="error-text">
              Email {emailError}
            </span>
          )}
        </div>
        <div className="field">
          <label>
            <span>
              Name <span className="muted">(optional)</span>
            </span>
            <input
              autoComplete="name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              aria-invalid={nameError ? true : undefined}
              aria-describedby={nameError ? `${id}-name-error` : undefined}
            />
          </label>
          {nameError && (
            <span id={`${id}-name-error`} className="error-text">
              Name {nameError}
            </span>
          )}
        </div>
        <button type="submit" disabled={subscribe.isPending || email.trim() === ''}>
          {subscribe.isPending ? 'Subscribing…' : 'Subscribe'}
        </button>
      </div>
      <ErrorText error={otherError} />
    </form>
  )
}
