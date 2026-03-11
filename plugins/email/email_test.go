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

// --- Plugin tests ---

func TestPlugin_Name(t *testing.T) {
	p := &Plugin{}
	assert.Equal(t, "email", p.Name())
}

func TestPlugin_Prefix(t *testing.T) {
	p := &Plugin{}
	assert.Equal(t, "email", p.Prefix())
}

func TestPlugin_HasServices(t *testing.T) {
	p := &Plugin{}
	assert.True(t, p.HasServices())
}

func TestPlugin_Nodes(t *testing.T) {
	p := &Plugin{}
	nodes := p.Nodes()
	require.Len(t, nodes, 1)
	assert.Equal(t, "send", nodes[0].Descriptor.Name())
}

func TestPlugin_Shutdown(t *testing.T) {
	p := &Plugin{}
	assert.NoError(t, p.Shutdown(nil))
}

// --- CreateService tests ---

func TestCreateService_Default(t *testing.T) {
	p := &Plugin{}
	svc, err := p.CreateService(map[string]any{
		"host": "smtp.example.com",
	})
	require.NoError(t, err)
	s := svc.(*Service)
	assert.Equal(t, "smtp.example.com", s.host)
	assert.Equal(t, 587, s.port) // default port
	assert.True(t, s.useTLS)     // default TLS
	assert.Empty(t, s.username)
	assert.Empty(t, s.password)
	assert.Empty(t, s.from)
}

func TestCreateService_MissingHost(t *testing.T) {
	p := &Plugin{}
	_, err := p.CreateService(map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing 'host'")
}

func TestCreateService_AllOptions(t *testing.T) {
	p := &Plugin{}
	svc, err := p.CreateService(map[string]any{
		"host":     "mail.example.com",
		"port":     float64(465),
		"username": "user",
		"password": "pass",
		"from":     "sender@example.com",
		"tls":      false,
	})
	require.NoError(t, err)
	s := svc.(*Service)
	assert.Equal(t, "mail.example.com", s.host)
	assert.Equal(t, 465, s.port)
	assert.Equal(t, "user", s.username)
	assert.Equal(t, "pass", s.password)
	assert.Equal(t, "sender@example.com", s.from)
	assert.False(t, s.useTLS)
}

func TestCreateService_PortAsInt(t *testing.T) {
	p := &Plugin{}
	svc, err := p.CreateService(map[string]any{
		"host": "smtp.example.com",
		"port": 25,
	})
	require.NoError(t, err)
	s := svc.(*Service)
	assert.Equal(t, 25, s.port)
}

// --- resolveRecipients tests ---

func TestResolveRecipients_MissingKey(t *testing.T) {
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	result, err := resolveRecipients(execCtx, map[string]any{}, "to")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestResolveRecipients_StringLiteral(t *testing.T) {
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	result, err := resolveRecipients(execCtx, map[string]any{
		"to": "user@example.com",
	}, "to")
	require.NoError(t, err)
	assert.Equal(t, []string{"user@example.com"}, result)
}

func TestResolveRecipients_StringResolvesToSliceAny(t *testing.T) {
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"recipients": []any{"a@example.com", "b@example.com"},
	}))
	result, err := resolveRecipients(execCtx, map[string]any{
		"to": "{{ input.recipients }}",
	}, "to")
	require.NoError(t, err)
	assert.Equal(t, []string{"a@example.com", "b@example.com"}, result)
}

func TestResolveRecipients_StringResolvesToStringSlice(t *testing.T) {
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"recipients": []string{"a@example.com", "b@example.com"},
	}))
	result, err := resolveRecipients(execCtx, map[string]any{
		"to": "{{ input.recipients }}",
	}, "to")
	require.NoError(t, err)
	assert.Equal(t, []string{"a@example.com", "b@example.com"}, result)
}

func TestResolveRecipients_StringResolvesToInvalidType(t *testing.T) {
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"recipients": 42,
	}))
	_, err := resolveRecipients(execCtx, map[string]any{
		"to": "{{ input.recipients }}",
	}, "to")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected string or []string")
}

func TestResolveRecipients_SliceAny(t *testing.T) {
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	result, err := resolveRecipients(execCtx, map[string]any{
		"to": []any{"a@example.com", "b@example.com"},
	}, "to")
	require.NoError(t, err)
	assert.Equal(t, []string{"a@example.com", "b@example.com"}, result)
}

func TestResolveRecipients_SliceString(t *testing.T) {
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	result, err := resolveRecipients(execCtx, map[string]any{
		"to": []string{"a@example.com"},
	}, "to")
	require.NoError(t, err)
	assert.Equal(t, []string{"a@example.com"}, result)
}

func TestResolveRecipients_InvalidType(t *testing.T) {
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	_, err := resolveRecipients(execCtx, map[string]any{
		"to": 123,
	}, "to")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid type")
}

// --- anySliceToStrings tests ---

func TestAnySliceToStrings_NonStringElement(t *testing.T) {
	_, err := anySliceToStrings([]any{"a@example.com", 42}, "to")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-string element")
}

func TestAnySliceToStrings_AllStrings(t *testing.T) {
	result, err := anySliceToStrings([]any{"a@example.com", "b@example.com"}, "to")
	require.NoError(t, err)
	assert.Equal(t, []string{"a@example.com", "b@example.com"}, result)
}

func TestAnySliceToStrings_Empty(t *testing.T) {
	result, err := anySliceToStrings([]any{}, "to")
	require.NoError(t, err)
	assert.Empty(t, result)
}

// --- Send: missing from ---

func TestSend_MissingFromBothConfigAndMessage(t *testing.T) {
	mock := newMockSMTP(t)
	defer mock.close()

	host, _, _ := net.SplitHostPort(mock.addr())
	svc := &Service{
		host:   host,
		from:   "", // no default from
		useTLS: false,
	}
	svc.dialFn = func() (*smtp.Client, error) {
		conn, err := net.Dial("tcp", mock.addr())
		if err != nil {
			return nil, err
		}
		return smtp.NewClient(conn, host)
	}

	msg := &Message{
		From:    "", // no per-message from
		To:      []string{"user@example.com"},
		Subject: "Test",
		Body:    "body",
	}
	_, err := svc.Send(msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing 'from' address")
}

// --- Send executor: missing from via executor ---

func TestSendExecutor_MissingFromNoDefault(t *testing.T) {
	mock := newMockSMTP(t)
	defer mock.close()

	host, _, _ := net.SplitHostPort(mock.addr())
	svc := &Service{
		host:   host,
		from:   "", // no default from
		useTLS: false,
	}
	svc.dialFn = func() (*smtp.Client, error) {
		conn, err := net.Dial("tcp", mock.addr())
		if err != nil {
			return nil, err
		}
		return smtp.NewClient(conn, host)
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
	assert.Contains(t, err.Error(), "missing 'from' address")
}

// --- Send executor: missing service ---

func TestSendExecutor_MissingService(t *testing.T) {
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newSendExecutor(nil)
	_, _, err := e.Execute(context.Background(), execCtx, map[string]any{
		"to":      "user@example.com",
		"subject": "Test",
		"body":    "test",
	}, map[string]any{})
	require.Error(t, err)
}

// --- Send executor: missing "to" ---

func TestSendExecutor_MissingTo(t *testing.T) {
	mock := newMockSMTP(t)
	defer mock.close()

	svc := newTestEmailService(t, mock)
	services := map[string]any{"mailer": svc}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newSendExecutor(nil)
	_, _, err := e.Execute(context.Background(), execCtx, map[string]any{
		"subject": "Test",
		"body":    "test",
	}, services)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field \"to\"")
}

// --- Descriptor tests ---

func TestSendDescriptor_Name(t *testing.T) {
	d := &sendDescriptor{}
	assert.Equal(t, "send", d.Name())
}

func TestSendDescriptor_ServiceDeps(t *testing.T) {
	d := &sendDescriptor{}
	deps := d.ServiceDeps()
	require.Contains(t, deps, "mailer")
	assert.Equal(t, "email", deps["mailer"].Prefix)
	assert.True(t, deps["mailer"].Required)
}

func TestSendDescriptor_ConfigSchema(t *testing.T) {
	d := &sendDescriptor{}
	schema := d.ConfigSchema()
	assert.Equal(t, "object", schema["type"])
	props := schema["properties"].(map[string]any)
	assert.Contains(t, props, "to")
	assert.Contains(t, props, "subject")
	assert.Contains(t, props, "body")
}

// --- SendExecutor Outputs ---

func TestSendExecutor_Outputs(t *testing.T) {
	e := newSendExecutor(nil)
	outputs := e.Outputs()
	assert.Contains(t, outputs, "success")
	assert.Contains(t, outputs, "error")
}

// --- Service SetDialFn ---

func TestService_SetDialFn(t *testing.T) {
	svc := &Service{host: "localhost", port: 25}
	called := false
	svc.SetDialFn(func() (*smtp.Client, error) {
		called = true
		return nil, fmt.Errorf("test dial")
	})
	_, err := svc.dial()
	assert.True(t, called)
	assert.Error(t, err)
}
