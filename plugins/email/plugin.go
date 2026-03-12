package email

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"

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

	port := 587
	if v, ok := config["port"].(float64); ok {
		port = int(v)
	} else if v, ok := config["port"].(int); ok {
		port = v
	}

	username, _ := config["username"].(string)
	password, _ := config["password"].(string)
	from, _ := config["from"].(string)

	useTLS := true
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
