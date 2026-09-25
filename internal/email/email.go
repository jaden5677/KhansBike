// Package email sends the system's transactional mail (today, only the
// mailing-list double opt-in confirmation). Two backends exist: LogSender for
// development, which writes the message to the log instead of sending it, and
// SMTPSender for production.
package email

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"log/slog"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// Message is a plain-text email.
type Message struct {
	To      mail.Address
	Subject string
	Text    string
}

// Sender delivers messages.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

// LogSender logs messages instead of sending them (EMAIL_BACKEND=log). The
// full body is logged so a developer can click the confirmation link.
type LogSender struct {
	Logger *slog.Logger
}

// Send implements Sender.
func (s LogSender) Send(ctx context.Context, m Message) error {
	s.Logger.InfoContext(ctx, "email not sent (EMAIL_BACKEND=log)", "to", m.To.Address, "subject", m.Subject, "body", m.Text)
	return nil
}

// SMTPSender delivers through an SMTP relay (EMAIL_BACKEND=smtp). Port 465
// uses implicit TLS; any other port must offer STARTTLS, which is required so
// credentials and addresses never cross the network in clear text.
type SMTPSender struct {
	Host     string
	Port     int
	Username string
	Password string
	From     mail.Address
}

// dialTimeout bounds connection setup; the whole exchange is also bounded by
// the caller's context deadline.
const dialTimeout = 15 * time.Second

// Send implements Sender.
func (s SMTPSender) Send(ctx context.Context, m Message) error {
	msg, err := s.compose(m)
	if err != nil {
		return err
	}
	addr := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	dialer := &net.Dialer{Timeout: dialTimeout}
	var conn net.Conn
	if s.Port == 465 {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: &tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12}}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("email: connect %s: %w", addr, err)
	}
	// net/smtp has no context support, so the deadline is applied to the
	// connection instead.
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	c, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("email: smtp handshake: %w", err)
	}
	defer func() { _ = c.Close() }()

	if s.Port != 465 {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return fmt.Errorf("email: %s does not offer STARTTLS", addr)
		}
		if err := c.StartTLS(&tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("email: starttls: %w", err)
		}
	}
	if s.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", s.Username, s.Password, s.Host)); err != nil {
			return fmt.Errorf("email: auth: %w", err)
		}
	}
	if err := c.Mail(s.From.Address); err != nil {
		return fmt.Errorf("email: MAIL FROM: %w", err)
	}
	if err := c.Rcpt(m.To.Address); err != nil {
		return fmt.Errorf("email: RCPT TO: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("email: DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		_ = w.Close()
		return fmt.Errorf("email: write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("email: finish body: %w", err)
	}
	return c.Quit()
}

// compose renders the RFC 5322 message: UTF-8 text, quoted-printable encoded
// so long lines and non-ASCII characters survive any relay.
func (s SMTPSender) compose(m Message) ([]byte, error) {
	for _, v := range []string{m.To.Address, m.Subject, s.From.Address} {
		if strings.ContainsAny(v, "\r\n") {
			return nil, fmt.Errorf("email: header value contains a line break") // header injection
		}
	}
	var buf bytes.Buffer
	domain := s.From.Address[strings.LastIndex(s.From.Address, "@")+1:]
	fmt.Fprintf(&buf, "From: %s\r\n", s.From.String())
	fmt.Fprintf(&buf, "To: %s\r\n", m.To.String())
	fmt.Fprintf(&buf, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", m.Subject))
	fmt.Fprintf(&buf, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(&buf, "Message-ID: <%s@%s>\r\n", randomID(), domain)
	buf.WriteString("MIME-Version: 1.0\r\n")
	buf.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	buf.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
	qp := quotedprintable.NewWriter(&buf)
	if _, err := qp.Write([]byte(m.Text)); err != nil {
		return nil, fmt.Errorf("email: encode body: %w", err)
	}
	if err := qp.Close(); err != nil {
		return nil, fmt.Errorf("email: encode body: %w", err)
	}
	return buf.Bytes(), nil
}

func randomID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
