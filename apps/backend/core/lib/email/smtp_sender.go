package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"sort"
	"strings"
	"time"
)

// dialTimeout bounds the TCP connect and convoTimeout the whole SMTP conversation - net/smtp.SendMail applies neither, so a slow or blackholed relay would otherwise pin the calling goroutine and connection for the OS default of a minute or two on every registration and password-reset request
const (
	dialTimeout  = 10 * time.Second
	convoTimeout = 30 * time.Second
)

// Delivers through net/smtp with PLAIN authentication - the minimum-viable production transport, working with SES, Postmark, Mailtrap, Gmail or any self-hosted relay - Swap for a provider SDK when you need webhooks, bounces or rate limiting
type SMTPSender struct {
	Host     string
	Port     string
	Username string
	Password string
}

// Composes a multipart/alternative message, text plus HTML, and hands it to the SMTP server
// With a username set it uses PLAIN auth, which net/smtp only allows over a TLS-upgraded connection, so the provider must advertise STARTTLS; with an empty username, as with a dev catcher like Mailpit, auth is skipped and the connection stays plaintext
func (s *SMTPSender) Send(ctx context.Context, msg Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	addr := s.Host + ":" + s.Port

	conn, err := (&net.Dialer{Timeout: dialTimeout}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("dial smtp: %w", err)
	}
	// A hard deadline on the entire conversation, read and write, clamped to the ctx deadline when that's sooner
	deadline := time.Now().Add(convoTimeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	_ = conn.SetDeadline(deadline)

	c, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("smtp client: %w", err)
	}
	defer c.Close()

	// Upgrade to TLS when the server advertises STARTTLS - real relays do, while dev catchers like Mailpit don't, keeping the plaintext path
	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: s.Host}); err != nil {
			return fmt.Errorf("starttls: %w", err)
		}
	}
	if s.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", s.Username, s.Password, s.Host)); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := c.Mail(extractAddr(msg.From)); err != nil {
		return fmt.Errorf("smtp mail: %w", err)
	}
	if err := c.Rcpt(msg.To); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(buildMIMEMessage(msg)); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close: %w", err)
	}
	return c.Quit()
}

// Builds a multipart/alternative email with both text and HTML parts - the boundary is a fixed dev-friendly string, which real deliverability infra would randomize, though it isn't correctness-critical
func buildMIMEMessage(msg Message) []byte {
	boundary := "skelet-mime-boundary"
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", msg.From)
	fmt.Fprintf(&b, "To: %s\r\n", msg.To)
	fmt.Fprintf(&b, "Subject: %s\r\n", msg.Subject)
	keys := make([]string, 0, len(msg.Headers))
	for k := range msg.Headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "%s: %s\r\n", headerSafe(k), headerSafe(msg.Headers[k]))
	}
	fmt.Fprintf(&b, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", boundary)

	fmt.Fprintf(&b, "--%s\r\n", boundary)
	fmt.Fprintf(&b, "Content-Type: text/plain; charset=UTF-8\r\n\r\n")
	fmt.Fprintf(&b, "%s\r\n", msg.TextBody)

	fmt.Fprintf(&b, "--%s\r\n", boundary)
	fmt.Fprintf(&b, "Content-Type: text/html; charset=UTF-8\r\n\r\n")
	fmt.Fprintf(&b, "%s\r\n", msg.HTMLBody)

	fmt.Fprintf(&b, "--%s--\r\n", boundary)
	return []byte(b.String())
}

// A CR or LF in a header value would let it start a header of its own
func headerSafe(s string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(s)
}

// Pulls the bare email address out of a "Display <addr@domain>" header - SMTP's MAIL FROM envelope wants just the address
func extractAddr(from string) string {
	if i := strings.LastIndex(from, "<"); i != -1 {
		if j := strings.LastIndex(from, ">"); j > i {
			return from[i+1 : j]
		}
	}
	return from
}
