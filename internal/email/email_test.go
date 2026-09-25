package email

import (
	"io"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"testing"
)

func TestComposeEncodesHeadersAndBody(t *testing.T) {
	s := SMTPSender{From: mail.Address{Name: "Khan's Bike Zone", Address: "hello@example.com"}}
	body := "Confirm here: https://example.com/subscribe/confirm?token=" + strings.Repeat("x", 90) + "\nCafé ✓"
	msg, err := s.compose(Message{To: mail.Address{Address: "rider@example.org"}, Subject: "Confirm your subscription ✓", Text: body})
	if err != nil {
		t.Fatalf("compose: %v", err)
	}
	head, encoded, ok := strings.Cut(string(msg), "\r\n\r\n")
	if !ok {
		t.Fatal("no header/body separator")
	}
	for _, want := range []string{"From: \"Khan's Bike Zone\" <hello@example.com>", "To: <rider@example.org>", "Subject: =?utf-8?q?", "Message-ID: <"} {
		if !strings.Contains(head, want) {
			t.Errorf("headers missing %q:\n%s", want, head)
		}
	}
	decoded, err := io.ReadAll(quotedprintable.NewReader(strings.NewReader(encoded)))
	if err != nil {
		t.Fatalf("decode body: %v", err)
	}
	// Mail lines end in CRLF on the wire; the text itself must survive intact.
	if got := strings.ReplaceAll(string(decoded), "\r\n", "\n"); got != body {
		t.Errorf("body round trip = %q, want %q", got, body)
	}
}

func TestComposeRejectsHeaderInjection(t *testing.T) {
	s := SMTPSender{From: mail.Address{Address: "hello@example.com"}}
	_, err := s.compose(Message{To: mail.Address{Address: "a@example.org"}, Subject: "hi\r\nBcc: victim@example.org", Text: "x"})
	if err == nil {
		t.Error("subject with CRLF was accepted")
	}
}
