package service

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/ArthurWerle/reports/internal/config"
	mail "github.com/wneessen/go-mail"
)

// Mailer sends the report as an HTML email with inline PNG charts.
type Mailer struct {
	cfg config.SMTPConfig
}

func NewMailer(cfg config.SMTPConfig) *Mailer {
	return &Mailer{cfg: cfg}
}

// ParseRecipients splits a comma-separated list and drops blanks.
func ParseRecipients(list string) []string {
	parts := strings.Split(list, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// Send delivers the HTML email. charts maps a chart name (e.g.
// "expenses-by-category") to its PNG bytes; each is embedded so the matching
// cid: reference in the HTML resolves.
func (m *Mailer) Send(subject, htmlBody string, recipients []string, charts map[string][]byte) error {
	port, err := strconv.Atoi(m.cfg.Port)
	if err != nil {
		return fmt.Errorf("invalid SMTP_PORT %q: %w", m.cfg.Port, err)
	}

	msg := mail.NewMsg()
	if err := msg.From(m.cfg.From); err != nil {
		return fmt.Errorf("set From: %w", err)
	}
	if err := msg.To(recipients...); err != nil {
		return fmt.Errorf("set To: %w", err)
	}
	msg.Subject(subject)
	msg.SetBodyString(mail.TypeTextHTML, htmlBody)

	for name, data := range charts {
		if len(data) == 0 {
			continue
		}
		if err := msg.EmbedReader(name+".png", bytes.NewReader(data), mail.WithFileContentID(name)); err != nil {
			return fmt.Errorf("embed chart %s: %w", name, err)
		}
	}

	opts := []mail.Option{mail.WithPort(port)}
	if m.cfg.Username != "" {
		opts = append(opts,
			mail.WithSMTPAuth(mail.SMTPAuthPlain),
			mail.WithUsername(m.cfg.Username),
			mail.WithPassword(m.cfg.Password),
		)
	} else {
		// No credentials (e.g. a local Mailpit relay): don't require STARTTLS.
		opts = append(opts, mail.WithTLSPolicy(mail.NoTLS))
	}

	client, err := mail.NewClient(m.cfg.Host, opts...)
	if err != nil {
		return fmt.Errorf("build SMTP client: %w", err)
	}
	if err := client.DialAndSend(msg); err != nil {
		return fmt.Errorf("send email: %w", err)
	}
	return nil
}
