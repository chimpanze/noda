package email

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"sync"
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSMTP is a minimal SMTP server for testing.
type mockSMTP struct {
	listener net.Listener
	messages []receivedMessage
	mu       sync.Mutex
	wg       sync.WaitGroup
}

type receivedMessage struct {
	from       string
	recipients []string
	data       string
}

func newMockSMTP(t *testing.T) *mockSMTP {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	m := &mockSMTP{listener: l}
	m.wg.Add(1)
	go m.serve(t)
	return m
}

func (m *mockSMTP) addr() string {
	return m.listener.Addr().String()
}

func (m *mockSMTP) close() {
	_ = m.listener.Close()
	m.wg.Wait()
}

func (m *mockSMTP) serve(t *testing.T) {
	defer m.wg.Done()
	for {
		conn, err := m.listener.Accept()
		if err != nil {
			return
		}
		m.wg.Add(1)
		go m.handleConn(t, conn)
	}
}

func (m *mockSMTP) handleConn(t *testing.T, conn net.Conn) {
	defer m.wg.Done()
	defer func() { _ = conn.Close() }()

	scanner := bufio.NewScanner(conn)
	_, _ = fmt.Fprintf(conn, "220 localhost ESMTP mock\r\n")

	var msg receivedMessage
	inData := false
	var dataBuilder strings.Builder

	for scanner.Scan() {
		line := scanner.Text()

		if inData {
			if line == "." {
				inData = false
				msg.data = dataBuilder.String()
				m.mu.Lock()
				m.messages = append(m.messages, msg)
				m.mu.Unlock()
				msg = receivedMessage{}
				_, _ = fmt.Fprintf(conn, "250 OK\r\n")
				continue
			}
			dataBuilder.WriteString(line + "\r\n")
			continue
		}

		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "EHLO") || strings.HasPrefix(upper, "HELO"):
			_, _ = fmt.Fprintf(conn, "250 localhost\r\n")
		case strings.HasPrefix(upper, "MAIL FROM:"):
			msg.from = extractAddr(line)
			_, _ = fmt.Fprintf(conn, "250 OK\r\n")
		case strings.HasPrefix(upper, "RCPT TO:"):
			msg.recipients = append(msg.recipients, extractAddr(line))
			_, _ = fmt.Fprintf(conn, "250 OK\r\n")
		case strings.HasPrefix(upper, "DATA"):
			inData = true
			dataBuilder.Reset()
			_, _ = fmt.Fprintf(conn, "354 Start mail input\r\n")
		case strings.HasPrefix(upper, "QUIT"):
			_, _ = fmt.Fprintf(conn, "221 Bye\r\n")
			return
		case strings.HasPrefix(upper, "RSET"):
			_, _ = fmt.Fprintf(conn, "250 OK\r\n")
		default:
			_, _ = fmt.Fprintf(conn, "500 Unrecognized command\r\n")
		}
	}
}

func extractAddr(line string) string {
	start := strings.Index(line, "<")
	end := strings.Index(line, ">")
	if start >= 0 && end > start {
		return line[start+1 : end]
	}
	parts := strings.SplitN(line, ":", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[1])
	}
	return line
}

func newTestEmailService(t *testing.T, mock *mockSMTP) *Service {
	t.Helper()
	host, portStr, _ := net.SplitHostPort(mock.addr())
	port := 0
	_, _ = fmt.Sscanf(portStr, "%d", &port)

	svc := &Service{
		host:   host,
		port:   port,
		from:   "test@example.com",
		useTLS: false,
	}
	// Override dial to use plain TCP
	svc.dialFn = func() (*smtp.Client, error) {
		conn, err := net.Dial("tcp", mock.addr())
		if err != nil {
			return nil, err
		}
		return smtp.NewClient(conn, host)
	}
	return svc
}

func TestSend_Basic(t *testing.T) {
	mock := newMockSMTP(t)
	defer mock.close()

	svc := newTestEmailService(t, mock)
	services := map[string]any{"mailer": svc}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"recipient": "user@example.com",
		"subject":   "Test Subject",
		"body":      "Hello, World!",
	}))

	e := newSendExecutor(nil)
	output, result, err := e.Execute(context.Background(), execCtx, map[string]any{
		"to":      "{{ input.recipient }}",
		"subject": "{{ input.subject }}",
		"body":    "{{ input.body }}",
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	data := result.(map[string]any)
	assert.NotEmpty(t, data["message_id"])

	mock.mu.Lock()
	defer mock.mu.Unlock()
	require.Len(t, mock.messages, 1)
	assert.Equal(t, "test@example.com", mock.messages[0].from)
	assert.Contains(t, mock.messages[0].recipients, "user@example.com")
	assert.Contains(t, mock.messages[0].data, "Test Subject")
	assert.Contains(t, mock.messages[0].data, "Hello, World!")
}

func TestSend_HTMLContent(t *testing.T) {
	mock := newMockSMTP(t)
	defer mock.close()

	svc := newTestEmailService(t, mock)
	services := map[string]any{"mailer": svc}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newSendExecutor(nil)
	output, _, err := e.Execute(context.Background(), execCtx, map[string]any{
		"to":           "user@example.com",
		"subject":      "HTML Test",
		"body":         "<h1>Hello</h1>",
		"content_type": "html",
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	mock.mu.Lock()
	defer mock.mu.Unlock()
	require.Len(t, mock.messages, 1)
	assert.Contains(t, mock.messages[0].data, "text/html")
	assert.Contains(t, mock.messages[0].data, "<h1>Hello</h1>")
}

func TestSend_PlainText(t *testing.T) {
	mock := newMockSMTP(t)
	defer mock.close()

	svc := newTestEmailService(t, mock)
	services := map[string]any{"mailer": svc}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newSendExecutor(nil)
	output, _, err := e.Execute(context.Background(), execCtx, map[string]any{
		"to":           "user@example.com",
		"subject":      "Plain Test",
		"body":         "Just text",
		"content_type": "text",
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	mock.mu.Lock()
	defer mock.mu.Unlock()
	require.Len(t, mock.messages, 1)
	assert.Contains(t, mock.messages[0].data, "text/plain")
}

func TestSend_MultipleRecipients(t *testing.T) {
	mock := newMockSMTP(t)
	defer mock.close()

	svc := newTestEmailService(t, mock)
	services := map[string]any{"mailer": svc}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newSendExecutor(nil)
	output, _, err := e.Execute(context.Background(), execCtx, map[string]any{
		"to":      []any{"alice@example.com", "bob@example.com"},
		"cc":      []any{"cc@example.com"},
		"bcc":     []any{"bcc@example.com"},
		"subject": "Multi",
		"body":    "Hello all",
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	mock.mu.Lock()
	defer mock.mu.Unlock()
	require.Len(t, mock.messages, 1)
	assert.Contains(t, mock.messages[0].recipients, "alice@example.com")
	assert.Contains(t, mock.messages[0].recipients, "bob@example.com")
	assert.Contains(t, mock.messages[0].recipients, "cc@example.com")
	assert.Contains(t, mock.messages[0].recipients, "bcc@example.com")
}

func TestSend_CustomFrom(t *testing.T) {
	mock := newMockSMTP(t)
	defer mock.close()

	svc := newTestEmailService(t, mock)
	services := map[string]any{"mailer": svc}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newSendExecutor(nil)
	output, _, err := e.Execute(context.Background(), execCtx, map[string]any{
		"to":      "user@example.com",
		"from":    "custom@example.com",
		"subject": "From test",
		"body":    "Custom sender",
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	mock.mu.Lock()
	defer mock.mu.Unlock()
	require.Len(t, mock.messages, 1)
	assert.Equal(t, "custom@example.com", mock.messages[0].from)
}

func TestSend_SMTPError(t *testing.T) {
	// Use a closed server to simulate SMTP error
	svc := &Service{
		host:   "127.0.0.1",
		port:   1,
		from:   "test@example.com",
		useTLS: false,
	}
	services := map[string]any{"mailer": svc}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newSendExecutor(nil)
	_, _, err := e.Execute(context.Background(), execCtx, map[string]any{
		"to":      "user@example.com",
		"subject": "Test",
		"body":    "test",
	}, services)
	require.Error(t, err)
}

func TestSend_ReplyTo(t *testing.T) {
	mock := newMockSMTP(t)
	defer mock.close()

	svc := newTestEmailService(t, mock)
	services := map[string]any{"mailer": svc}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newSendExecutor(nil)
	output, _, err := e.Execute(context.Background(), execCtx, map[string]any{
		"to":       "user@example.com",
		"subject":  "Reply-To test",
		"body":     "body",
		"reply_to": "reply@example.com",
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	mock.mu.Lock()
	defer mock.mu.Unlock()
	require.Len(t, mock.messages, 1)
	assert.Contains(t, mock.messages[0].data, "Reply-To: reply@example.com")
}
