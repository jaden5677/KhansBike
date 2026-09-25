package service

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"text/template"
	"time"
	"unicode"

	"github.com/google/uuid"

	"github.com/khansbikezone/bikezone-api/internal/auth"
	"github.com/khansbikezone/bikezone-api/internal/domain"
	"github.com/khansbikezone/bikezone-api/internal/email"
	"github.com/khansbikezone/bikezone-api/internal/jobs"
	"github.com/khansbikezone/bikezone-api/internal/store"
	"github.com/khansbikezone/bikezone-api/internal/store/gen"
)

// Subscribers runs the optional customer mailing list with double opt-in: a
// signup is pending until the emailed confirmation link is used.
//
// Raw tokens never touch the database. Signing up only records the address
// and enqueues a job; the job mints a fresh confirm/unsubscribe token pair,
// stores their hashes, and emails the raw values. A retried send simply mints
// a new pair (the undelivered one was never seen by anyone).
type Subscribers struct {
	store         *store.Store
	sender        email.Sender
	publicBaseURL string
	now           func() time.Time
}

// NewSubscribers builds the mailing-list service. publicBaseURL is where the
// web app serves the /subscribe/confirm and /subscribe/unsubscribe pages that
// the emailed links open.
func NewSubscribers(st *store.Store, sender email.Sender, publicBaseURL string) *Subscribers {
	return &Subscribers{store: st, sender: sender, publicBaseURL: strings.TrimSuffix(publicBaseURL, "/"), now: time.Now}
}

const (
	confirmTTL = 48 * time.Hour
	// resendCooldown stops the signup form from being used to flood someone's
	// inbox: a repeat signup within this window sends nothing new.
	resendCooldown = 10 * time.Minute
	maxEmailLen    = 254 // RFC 5321 path limit
	exportPageSize = 500
)

var sourcePattern = regexp.MustCompile(`^[a-z0-9_-]{1,50}$`)

// SubscribeInput is a public signup.
type SubscribeInput struct {
	Email  string  `json:"email"`
	Name   *string `json:"name"`
	Source *string `json:"source"` // where the form was, e.g. "footer"
}

// Subscribe records a signup and, for a pending address, schedules the
// confirmation email. The outcome is deliberately invisible to the caller
// (confirmed, pending and new addresses all succeed identically) so the form
// cannot be used to discover who is subscribed.
func (s *Subscribers) Subscribe(ctx context.Context, in SubscribeInput) error {
	v := &domain.ValidationError{}
	address := strings.TrimSpace(in.Email)
	if parsed, err := mail.ParseAddress(address); err != nil || parsed.Address != address || len(address) > maxEmailLen {
		v.Add("email", "is not a valid email address")
	}
	name := optionalText(v, "name", in.Name, maxNameLen)
	if name != nil && strings.ContainsFunc(*name, unicode.IsControl) {
		v.Add("name", "must not contain control characters")
	}
	source := optionalText(v, "source", in.Source, 50)
	if source != nil && !sourcePattern.MatchString(*source) {
		v.Add("source", "must be lowercase letters, digits, _ or -")
	}
	if err := v.Err(); err != nil {
		return err
	}
	return s.store.InTx(ctx, func(q *store.Queries) error {
		row, err := q.UpsertSubscriber(ctx, gen.UpsertSubscriberParams{
			ID: domain.NewID(), Email: address, Name: store.Text(name), Source: store.Text(source),
		})
		if err != nil {
			return err
		}
		if row.Status != gen.SubscriberStatusPending {
			return nil
		}
		now := s.now()
		claimed, err := q.ClaimConfirmationSend(ctx, gen.ClaimConfirmationSendParams{
			ID:            row.ID,
			ExpiresAt:     store.Timestamptz(now.Add(confirmTTL)),
			CooldownUntil: store.Timestamptz(now.Add(confirmTTL - resendCooldown)),
		})
		if err != nil || claimed == 0 {
			return err // claimed == 0: a confirmation went out moments ago
		}
		return q.Enqueue(ctx, JobSendConfirmation, confirmationPayload{SubscriberID: row.ID})
	})
}

type confirmationPayload struct {
	SubscriberID uuid.UUID `json:"subscriberId"`
}

var confirmationEmail = template.Must(template.New("confirm").Parse(`Hi {{.Name}},

Please confirm that you would like to receive news and offers from Khan's Bike Zone:

{{.ConfirmURL}}

This link expires in 48 hours. If you did not sign up, ignore this email and
you will not hear from us again, or remove your address now:

{{.UnsubscribeURL}}
`))

// SendConfirmationJob is the send_subscriber_confirmation job handler.
func (s *Subscribers) SendConfirmationJob(ctx context.Context, payload []byte) error {
	var p confirmationPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return jobs.Permanent(fmt.Errorf("decode payload: %w", err))
	}
	confirm, err := auth.NewToken()
	if err != nil {
		return err
	}
	unsubscribe, err := auth.NewToken()
	if err != nil {
		return err
	}
	row, err := s.store.IssueSubscriberTokens(ctx, gen.IssueSubscriberTokensParams{
		ID:                   p.SubscriberID,
		ConfirmTokenHash:     auth.HashToken(confirm),
		ConfirmExpiresAt:     store.Timestamptz(s.now().Add(confirmTTL)),
		UnsubscribeTokenHash: auth.HashToken(unsubscribe),
	})
	if errors.Is(err, domain.ErrNotFound) {
		return nil // confirmed or unsubscribed in the meantime: nothing to send
	}
	if err != nil {
		return err
	}

	name := "there"
	if row.Name.Valid {
		name = row.Name.String
	}
	var body strings.Builder
	if err := confirmationEmail.Execute(&body, map[string]string{
		"Name":           name,
		"ConfirmURL":     s.link("/subscribe/confirm", confirm),
		"UnsubscribeURL": s.link("/subscribe/unsubscribe", unsubscribe),
	}); err != nil {
		return jobs.Permanent(fmt.Errorf("render confirmation email: %w", err))
	}
	return s.sender.Send(ctx, email.Message{
		To:      mail.Address{Name: deref(store.StringPtr(row.Name)), Address: row.Email},
		Subject: "Please confirm your subscription to Khan's Bike Zone",
		Text:    body.String(),
	})
}

// link builds an emailed link to a web app page. The pages ask the visitor to
// press a button that POSTs the token to the API, because mail scanners
// prefetch links: a GET that confirmed on its own would let a scanner confirm
// a subscription nobody asked for.
func (s *Subscribers) link(path, token string) string {
	return s.publicBaseURL + path + "?token=" + url.QueryEscape(token)
}

var errBadToken = fmt.Errorf("%w: the link is invalid or has expired", domain.ErrNotFound)

// Confirm completes double opt-in for the address the token was sent to.
func (s *Subscribers) Confirm(ctx context.Context, token string) error {
	if strings.TrimSpace(token) == "" {
		return domain.Invalid("token", "is required")
	}
	_, err := s.store.ConfirmSubscriber(ctx, auth.HashToken(token))
	if errors.Is(err, domain.ErrNotFound) {
		return errBadToken
	}
	return err
}

// Unsubscribe removes an address from the list. It is idempotent and
// reports success for unknown tokens too, revealing nothing.
func (s *Subscribers) Unsubscribe(ctx context.Context, token string) error {
	if strings.TrimSpace(token) == "" {
		return domain.Invalid("token", "is required")
	}
	_, err := s.store.UnsubscribeByToken(ctx, auth.HashToken(token))
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	return err
}

// Stats counts subscribers by status.
func (s *Subscribers) Stats(ctx context.Context) (map[domain.SubscriberStatus]int, error) {
	rows, err := s.store.CountSubscribersByStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("count subscribers: %w", err)
	}
	out := map[domain.SubscriberStatus]int{
		domain.SubscriberPending: 0, domain.SubscriberConfirmed: 0, domain.SubscriberUnsubscribed: 0,
	}
	for _, r := range rows {
		out[domain.SubscriberStatus(r.Status)] = int(r.Total)
	}
	return out, nil
}

// ExportConfirmed writes every confirmed subscriber as CSV, a page at a time
// so the list never has to fit in memory.
func (s *Subscribers) ExportConfirmed(ctx context.Context, w io.Writer) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"email", "name", "source", "confirmed_at", "signed_up_at"}); err != nil {
		return err
	}
	afterAt, afterID := time.Time{}, uuid.Nil
	for {
		rows, err := s.store.ListConfirmedSubscribers(ctx, gen.ListConfirmedSubscribersParams{
			AfterCreatedAt: store.Timestamptz(afterAt), AfterID: afterID, RowLimit: exportPageSize,
		})
		if err != nil {
			return fmt.Errorf("list subscribers: %w", err)
		}
		for _, r := range rows {
			confirmed := ""
			if r.ConfirmedAt.Valid {
				confirmed = r.ConfirmedAt.Time.UTC().Format(time.RFC3339)
			}
			if err := cw.Write([]string{
				csvCell(r.Email), csvCell(r.Name.String), csvCell(r.Source.String),
				confirmed, r.CreatedAt.Time.UTC().Format(time.RFC3339),
			}); err != nil {
				return err
			}
		}
		cw.Flush()
		if err := cw.Error(); err != nil {
			return err
		}
		if len(rows) < exportPageSize {
			return nil
		}
		last := rows[len(rows)-1]
		afterAt, afterID = last.CreatedAt.Time, last.ID
	}
}

// csvCell neutralises spreadsheet formula injection. Names come from a public
// form, and a cell such as =HYPERLINK(...) would execute when the owner opens
// the export in Excel; a leading apostrophe makes it plain text.
func csvCell(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}
