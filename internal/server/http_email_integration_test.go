package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/smtp"
	"strings"
	"sync"
	"testing"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/registry"
	emailplugin "github.com/chimpanze/noda/plugins/email"
	httpplugin "github.com/chimpanze/noda/plugins/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestE2E_OutboundHTTPCall tests: incoming webhook → workflow makes outbound HTTP call → uses response.
func TestE2E_OutboundHTTPCall(t *testing.T) {
	// External API mock
	externalAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"user": "alice",
			"role": "admin",
		})
	}))
	defer externalAPI.Close()

	// Create HTTP service
	hp := &httpplugin.Plugin{}
	rawHTTPSvc, err := hp.CreateService(map[string]any{
		"timeout": float64(10),
	})
	require.NoError(t, err)

	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("api-client", rawHTTPSvc, hp))

	nodeReg := buildTestNodeRegistry()
	_ = nodeReg.RegisterFromPlugin(&httpplugin.Plugin{})

	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Routes: map[string]map[string]any{
			"fetch-user": {
				"method": "GET",
				"path":   "/api/fetch-user",
				"trigger": map[string]any{
					"workflow": "fetch-user-wf",
				},
			},
		},
		Workflows: map[string]map[string]any{
			"fetch-user-wf": {
				"nodes": map[string]any{
					"fetch": map[string]any{
						"type":     "http.get",
						"services": map[string]any{"client": "api-client"},
						"config": map[string]any{
							"url": externalAPI.URL + "/users/me",
							"headers": map[string]any{
								"Authorization": "Bearer test-token",
							},
						},
					},
					"respond": map[string]any{
						"type":   "response.json",
						"config": map[string]any{"status": "200", "body": "{{ nodes.fetch }}"},
					},
				},
				"edges": []any{
					map[string]any{"from": "fetch", "to": "respond"},
				},
			},
		},
		Schemas: map[string]map[string]any{},
	}

	srv, err := NewServer(rc, svcReg, nodeReg)
	require.NoError(t, err)
	require.NoError(t, srv.Setup())

	req := httptest.NewRequest("GET", "/api/fetch-user", nil)
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(respBody, &result))

	// The response should contain the outbound HTTP call result
	assert.Equal(t, float64(200), result["status"])
	body := result["body"].(map[string]any)
	assert.Equal(t, "alice", body["user"])
	assert.Equal(t, "admin", body["role"])
}

// TestE2E_HTTPPostOutbound tests: POST request → outbound HTTP POST → response.
func TestE2E_HTTPPostOutbound(t *testing.T) {
	externalAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		bodyBytes, _ := io.ReadAll(r.Body)
		var payload map[string]any
		require.NoError(t, json.Unmarshal(bodyBytes, &payload))
		assert.Equal(t, "test-data", payload["data"])

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"created": true})
	}))
	defer externalAPI.Close()

	hp := &httpplugin.Plugin{}
	rawHTTPSvc, err := hp.CreateService(map[string]any{})
	require.NoError(t, err)

	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("api-client", rawHTTPSvc, hp))

	nodeReg := buildTestNodeRegistry()
	_ = nodeReg.RegisterFromPlugin(&httpplugin.Plugin{})

	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Routes: map[string]map[string]any{
			"post-data": {
				"method": "POST",
				"path":   "/api/post-data",
				"trigger": map[string]any{
					"workflow": "post-data-wf",
					"input": map[string]any{
						"payload": "{{ body }}",
					},
				},
			},
		},
		Workflows: map[string]map[string]any{
			"post-data-wf": {
				"nodes": map[string]any{
					"send": map[string]any{
						"type":     "http.post",
						"services": map[string]any{"client": "api-client"},
						"config": map[string]any{
							"url":  externalAPI.URL + "/create",
							"body": "{{ input.payload }}",
						},
					},
					"respond": map[string]any{
						"type":   "response.json",
						"config": map[string]any{"status": "200", "body": "{{ nodes.send }}"},
					},
				},
				"edges": []any{
					map[string]any{"from": "send", "to": "respond"},
				},
			},
		},
		Schemas: map[string]map[string]any{},
	}

	srv, err := NewServer(rc, svcReg, nodeReg)
	require.NoError(t, err)
	require.NoError(t, srv.Setup())

	payload, _ := json.Marshal(map[string]any{"data": "test-data"})
	req := httptest.NewRequest("POST", "/api/post-data", strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(respBody, &result))
	assert.Equal(t, float64(200), result["status"])
	body := result["body"].(map[string]any)
	assert.Equal(t, true, body["created"])
}

// TestE2E_HTTPTimeout tests: outbound HTTP timeout → error handling.
func TestE2E_HTTPTimeout(t *testing.T) {
	slowAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done() // block until client disconnects
	}))
	defer slowAPI.Close()

	hp := &httpplugin.Plugin{}
	rawHTTPSvc, err := hp.CreateService(map[string]any{})
	require.NoError(t, err)

	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("api-client", rawHTTPSvc, hp))

	nodeReg := buildTestNodeRegistry()
	_ = nodeReg.RegisterFromPlugin(&httpplugin.Plugin{})

	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Routes: map[string]map[string]any{
			"slow-call": {
				"method": "GET",
				"path":   "/api/slow",
				"trigger": map[string]any{
					"workflow": "slow-wf",
				},
			},
		},
		Workflows: map[string]map[string]any{
			"slow-wf": {
				"nodes": map[string]any{
					"fetch": map[string]any{
						"type":     "http.get",
						"services": map[string]any{"client": "api-client"},
						"config": map[string]any{
							"url":     slowAPI.URL,
							"timeout": "50ms",
						},
					},
					"on-error": map[string]any{
						"type": "response.error",
						"config": map[string]any{
							"status":  "504",
							"message": "upstream timeout",
						},
					},
					"respond": map[string]any{
						"type":   "response.json",
						"config": map[string]any{"status": "200", "body": "{{ nodes.fetch }}"},
					},
				},
				"edges": []any{
					map[string]any{"from": "fetch", "to": "respond"},
					map[string]any{"from": "fetch", "to": "on-error", "output": "error"},
				},
			},
		},
		Schemas: map[string]map[string]any{},
	}

	srv, err := NewServer(rc, svcReg, nodeReg)
	require.NoError(t, err)
	require.NoError(t, srv.Setup())

	req := httptest.NewRequest("GET", "/api/slow", nil)
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 504, resp.StatusCode)
}

// mockSMTPServer is a minimal SMTP server for integration tests.
type mockSMTPServer struct {
	listener net.Listener
	messages []mockSMTPMessage
	mu       sync.Mutex
	wg       sync.WaitGroup
}

type mockSMTPMessage struct {
	from       string
	recipients []string
	data       string
}

func newMockSMTPServer(t *testing.T) *mockSMTPServer {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	m := &mockSMTPServer{listener: l}
	m.wg.Add(1)
	go m.serve()
	return m
}

func (m *mockSMTPServer) addr() string { return m.listener.Addr().String() }

func (m *mockSMTPServer) close() {
	m.listener.Close()
	m.wg.Wait()
}

func (m *mockSMTPServer) serve() {
	defer m.wg.Done()
	for {
		conn, err := m.listener.Accept()
		if err != nil {
			return
		}
		m.wg.Add(1)
		go m.handleConn(conn)
	}
}

func (m *mockSMTPServer) handleConn(conn net.Conn) {
	defer m.wg.Done()
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	fmt.Fprintf(conn, "220 localhost ESMTP mock\r\n")

	var msg mockSMTPMessage
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
				msg = mockSMTPMessage{}
				fmt.Fprintf(conn, "250 OK\r\n")
				continue
			}
			dataBuilder.WriteString(line + "\r\n")
			continue
		}
		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
			fmt.Fprintf(conn, "250 localhost\r\n")
		case strings.HasPrefix(upper, "MAIL FROM:"):
			msg.from = extractSMTPAddr(line)
			fmt.Fprintf(conn, "250 OK\r\n")
		case strings.HasPrefix(upper, "RCPT TO:"):
			msg.recipients = append(msg.recipients, extractSMTPAddr(line))
			fmt.Fprintf(conn, "250 OK\r\n")
		case strings.HasPrefix(upper, "DATA"):
			inData = true
			dataBuilder.Reset()
			fmt.Fprintf(conn, "354 Start mail input\r\n")
		case strings.HasPrefix(upper, "QUIT"):
			fmt.Fprintf(conn, "221 Bye\r\n")
			return
		case strings.HasPrefix(upper, "RSET"):
			fmt.Fprintf(conn, "250 OK\r\n")
		default:
			fmt.Fprintf(conn, "500 Unrecognized\r\n")
		}
	}
}

func extractSMTPAddr(line string) string {
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

// TestE2E_EmailSend tests: webhook → workflow sends email.
func TestE2E_EmailSend(t *testing.T) {
	mock := newMockSMTPServer(t)
	defer mock.close()

	host, portStr, _ := net.SplitHostPort(mock.addr())
	var port int
	fmt.Sscanf(portStr, "%d", &port)

	ep := &emailplugin.Plugin{}
	rawEmailSvc, err := ep.CreateService(map[string]any{
		"host": host,
		"port": float64(port),
		"from": "noreply@example.com",
		"tls":  false,
	})
	require.NoError(t, err)

	// Override dial for plain TCP
	emailSvc := rawEmailSvc.(*emailplugin.Service)
	emailSvc.SetDialFn(func() (*smtp.Client, error) {
		conn, err := net.Dial("tcp", mock.addr())
		if err != nil {
			return nil, err
		}
		return smtp.NewClient(conn, host)
	})

	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("mailer", rawEmailSvc, ep))

	nodeReg := buildTestNodeRegistry()
	_ = nodeReg.RegisterFromPlugin(&emailplugin.Plugin{})

	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Routes: map[string]map[string]any{
			"send-email": {
				"method": "POST",
				"path":   "/api/send-email",
				"trigger": map[string]any{
					"workflow": "email-wf",
					"input": map[string]any{
						"email":   "{{ body.email }}",
						"subject": "{{ body.subject }}",
						"message": "{{ body.message }}",
					},
				},
			},
		},
		Workflows: map[string]map[string]any{
			"email-wf": {
				"nodes": map[string]any{
					"send": map[string]any{
						"type":     "email.send",
						"services": map[string]any{"mailer": "mailer"},
						"config": map[string]any{
							"to":           "{{ input.email }}",
							"subject":      "{{ input.subject }}",
							"body":         "{{ input.message }}",
							"content_type": "html",
						},
					},
					"respond": map[string]any{
						"type":   "response.json",
						"config": map[string]any{"status": "200", "body": "{{ nodes.send }}"},
					},
				},
				"edges": []any{
					map[string]any{"from": "send", "to": "respond"},
				},
			},
		},
		Schemas: map[string]map[string]any{},
	}

	srv, err := NewServer(rc, svcReg, nodeReg)
	require.NoError(t, err)
	require.NoError(t, srv.Setup())

	payload, _ := json.Marshal(map[string]any{
		"email":   "user@test.com",
		"subject": "Welcome!",
		"message": "<h1>Hello</h1>",
	})
	req := httptest.NewRequest("POST", "/api/send-email", strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(respBody, &result))
	assert.NotEmpty(t, result["message_id"])

	// Verify email was received by mock SMTP
	mock.mu.Lock()
	defer mock.mu.Unlock()
	require.Len(t, mock.messages, 1)
	assert.Equal(t, "noreply@example.com", mock.messages[0].from)
	assert.Contains(t, mock.messages[0].recipients, "user@test.com")
	assert.Contains(t, mock.messages[0].data, "Welcome!")
	assert.Contains(t, mock.messages[0].data, "<h1>Hello</h1>")
}
