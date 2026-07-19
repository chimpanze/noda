package email

import (
	"crypto/tls"
	"fmt"
	"math"
	"net"
	"net/smtp"
	"strconv"
	"strings"

	"github.com/chimpanze/noda/pkg/api"
)

// Plugin implements the SMTP email plugin.
type Plugin struct{}

func (p *Plugin) Name() string   { return "email" }
func (p *Plugin) Prefix() string { return "email" }

func (p *Plugin) HasServices() bool { return true }

func (p *Plugin) Nodes() []api.NodeRegistration {
	return []api.NodeRegistration{
		{Descriptor: &sendDescriptor{}, Factory: newSendExecutor},
	}
}

func (p *Plugin) CreateService(config map[string]any) (any, error) {
	host, _ := config["host"].(string)
	if host == "" {
		return nil, fmt.Errorf("email: missing 'host'")
	}

	port, err := parsePort(config["port"])
	if err != nil {
		return nil, err
	}

	username, _ := config["username"].(string)
	password, _ := config["password"].(string)
	from, _ := config["from"].(string)

	// Default useTLS: true only for port 465 (implicit TLS / SMTPS).
	// For all other ports (25, 587, custom), default to false so that
	// plaintext SMTP servers (e.g. Mailpit on 1025) are reachable.
	// An explicit "tls" config key always takes precedence.
	useTLS := port == 465
	if v, ok := config["tls"].(bool); ok {
		useTLS = v
	}

	return &Service{
		host:     host,
		port:     port,
		username: username,
		password: password,
		from:     from,
		useTLS:   useTLS,
	}, nil
}

// parsePort parses the "port" config value. $env() substitution always
// produces strings, so string values must be parsed — silently falling back
// to the default would ignore the operator's SMTP_PORT (issue #334).
// A nil value (key absent) returns the default 587.
func parsePort(raw any) (int, error) {
	port := 587
	switch v := raw.(type) {
	case nil:
	case float64:
		if v != math.Trunc(v) {
			return 0, fmt.Errorf("email: invalid port %v: must be an integer", v)
		}
		port = int(v)
	case int:
		port = v
	case int64:
		port = int(v)
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0, fmt.Errorf("email: invalid port %q: not a number (is the environment variable set?)", v)
		}
		port = n
	default:
		return 0, fmt.Errorf("email: invalid port: expected number or numeric string, got %T", raw)
	}
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("email: invalid port %d: must be in [1, 65535]", port)
	}
	return port, nil
}

// ServiceConfigSchema documents the email service `config` block. Every key
// here is read by CreateService/parsePort. additionalProperties is false:
// unknown keys are silently ignored by CreateService.
func (p *Plugin) ServiceConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"host": map[string]any{
				"type":        "string",
				"description": "SMTP server hostname; required — CreateService errors without it",
			},
			"port": map[string]any{
				"type":        []any{"string", "number"},
				"description": "SMTP port, as a number or numeric string ($env() always produces strings) (default 587)",
			},
			"username": map[string]any{
				"type":        "string",
				"description": "SMTP auth username",
			},
			"password": map[string]any{
				"type":        "string",
				"description": "SMTP auth password",
			},
			"from": map[string]any{
				"type":        "string",
				"description": "Default From address for outgoing mail",
			},
			"tls": map[string]any{
				"type":        "boolean",
				"description": "Use implicit TLS (SMTPS); defaults to true only when port is 465",
			},
		},
		"required":             []any{"host"},
		"additionalProperties": false,
	}
}

func (p *Plugin) HealthCheck(service any) error {
	svc, ok := service.(*Service)
	if !ok {
		return fmt.Errorf("email: invalid service type")
	}
	addr := net.JoinHostPort(svc.host, fmt.Sprintf("%d", svc.port))

	var conn net.Conn
	var err error
	if svc.useTLS {
		conn, err = tls.Dial("tcp", addr, &tls.Config{ServerName: svc.host})
	} else {
		conn, err = net.Dial("tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("email: health check: %w", err)
	}

	// smtp.NewClient takes ownership of conn; if it fails, we close conn ourselves.
	client, err := smtp.NewClient(conn, svc.host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("email: health check: %w", err)
	}
	// Quit sends QUIT and closes the underlying connection.
	return client.Quit()
}

func (p *Plugin) Shutdown(_ any) error {
	return nil
}
