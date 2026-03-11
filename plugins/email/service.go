package email

import (
	"crypto/tls"
	"fmt"
	"net/smtp"
	"strings"
)

// Service manages SMTP email sending.
type Service struct {
	host     string
	port     int
	username string
	password string
	from     string
	useTLS   bool

	// dialFn allows overriding the dialer for testing.
	dialFn func() (*smtp.Client, error)
}

// Send sends an email message.
func (s *Service) Send(msg *Message) (string, error) {
	client, err := s.dial()
	if err != nil {
		return "", fmt.Errorf("email: connect: %w", err)
	}
	defer func() { _ = client.Close() }()

	// Auth if credentials provided
	if s.username != "" {
		auth := smtp.PlainAuth("", s.username, s.password, s.host)
		if err := client.Auth(auth); err != nil {
			return "", fmt.Errorf("email: auth: %w", err)
		}
	}

	from := msg.From
	if from == "" {
		from = s.from
	}
	if from == "" {
		return "", fmt.Errorf("email: missing 'from' address")
	}

	if err := client.Mail(from); err != nil {
		return "", fmt.Errorf("email: MAIL FROM: %w", err)
	}

	// All recipients
	allRecipients := make([]string, 0, len(msg.To)+len(msg.CC)+len(msg.BCC))
	allRecipients = append(allRecipients, msg.To...)
	allRecipients = append(allRecipients, msg.CC...)
	allRecipients = append(allRecipients, msg.BCC...)

	for _, rcpt := range allRecipients {
		if err := client.Rcpt(rcpt); err != nil {
			return "", fmt.Errorf("email: RCPT TO %s: %w", rcpt, err)
		}
	}

	// Write message data
	wc, err := client.Data()
	if err != nil {
		return "", fmt.Errorf("email: DATA: %w", err)
	}

	// Build headers
	headers := make(map[string]string)
	headers["From"] = from
	headers["To"] = strings.Join(msg.To, ", ")
	headers["Subject"] = msg.Subject

	if len(msg.CC) > 0 {
		headers["Cc"] = strings.Join(msg.CC, ", ")
	}
	if msg.ReplyTo != "" {
		headers["Reply-To"] = msg.ReplyTo
	}

	contentType := "text/html; charset=UTF-8"
	if msg.ContentType == "text" {
		contentType = "text/plain; charset=UTF-8"
	}
	headers["Content-Type"] = contentType
	headers["MIME-Version"] = "1.0"

	// Generate message ID
	messageID := fmt.Sprintf("<%s@%s>", generateID(), s.host)
	headers["Message-ID"] = messageID

	var sb strings.Builder
	for k, v := range headers {
		fmt.Fprintf(&sb, "%s: %s\r\n", k, v)
	}
	sb.WriteString("\r\n")
	sb.WriteString(msg.Body)

	if _, err := wc.Write([]byte(sb.String())); err != nil {
		_ = wc.Close()
		return "", fmt.Errorf("email: write body: %w", err)
	}
	if err := wc.Close(); err != nil {
		return "", fmt.Errorf("email: close data: %w", err)
	}

	_ = client.Quit() // Non-fatal: message was already sent

	return messageID, nil
}

// SetDialFn overrides the default dialer for testing.
func (s *Service) SetDialFn(fn func() (*smtp.Client, error)) {
	s.dialFn = fn
}

func (s *Service) dial() (*smtp.Client, error) {
	if s.dialFn != nil {
		return s.dialFn()
	}

	addr := fmt.Sprintf("%s:%d", s.host, s.port)

	if s.useTLS {
		conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: s.host})
		if err != nil {
			return nil, err
		}
		return smtp.NewClient(conn, s.host)
	}

	return smtp.Dial(addr)
}

// Message represents an email message.
type Message struct {
	From        string
	To          []string
	CC          []string
	BCC         []string
	ReplyTo     string
	Subject     string
	Body        string
	ContentType string // "text" or "html"
}
