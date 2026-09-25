import { useEffect, useRef } from 'react'
import { Link, useSearchParams } from 'react-router'
import { ApiError } from '../api/client'
import { useConfirmSubscription, useUnsubscribe } from '../api/public'
import { ErrorText } from '../components/ErrorText'
import { SubscribeForm } from '../components/SubscribeForm'

function isDeadLink(error: unknown): boolean {
  return error instanceof ApiError && (error.status === 404 || error.status === 422)
}

/**
 * Opened from the confirmation email. Confirming is what the visitor came to
 * do, so it happens as soon as the page opens.
 */
export function ConfirmPage() {
  const [params] = useSearchParams()
  const token = params.get('token') ?? ''
  const confirm = useConfirmSubscription()

  // A ref survives React's development-mode double run of effects, so the
  // one-time token is sent once; a second send would fail as "already used".
  const sent = useRef(false)
  const { mutate } = confirm
  useEffect(() => {
    if (token && !sent.current) {
      sent.current = true
      mutate(token)
    }
  }, [token, mutate])

  return (
    <section className="narrow">
      <title>Confirm subscription | Khan's Bike Zone</title>
      <h1>Mailing list</h1>
      {!token || isDeadLink(confirm.error) ? (
        <>
          <p>This confirmation link has expired or was already used. You can sign up again below.</p>
          <SubscribeForm source="confirm-page" />
        </>
      ) : confirm.isSuccess ? (
        <p role="status">
          You're subscribed. Thanks! <Link to="/">Browse the shop</Link>
        </p>
      ) : confirm.isError ? (
        <>
          <ErrorText error={confirm.error} />
          <button type="button" onClick={() => mutate(token)}>
            Try again
          </button>
        </>
      ) : (
        <p role="status" className="muted">
          Confirming…
        </p>
      )}
    </section>
  )
}

/**
 * Opened from the unsubscribe link in every email. It waits for a button
 * press: email security scanners open links automatically, and one must not
 * be able to unsubscribe someone just by looking at their mail.
 */
export function UnsubscribePage() {
  const [params] = useSearchParams()
  const token = params.get('token') ?? ''
  const unsubscribe = useUnsubscribe()

  return (
    <section className="narrow">
      <title>Unsubscribe | Khan's Bike Zone</title>
      <h1>Unsubscribe</h1>
      {!token || isDeadLink(unsubscribe.error) ? (
        <p>This unsubscribe link is not valid. If you still get our emails, use the link in the latest one.</p>
      ) : unsubscribe.isSuccess ? (
        <p role="status">You've been removed from our mailing list. You won't get any more emails from us.</p>
      ) : (
        <>
          <p>Stop getting news and offers from Khan's Bike Zone?</p>
          <button type="button" disabled={unsubscribe.isPending} onClick={() => unsubscribe.mutate(token)}>
            {unsubscribe.isPending ? 'Unsubscribing…' : 'Unsubscribe'}
          </button>
          <ErrorText error={unsubscribe.error} />
        </>
      )}
    </section>
  )
}
