//go:build integration

package email

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/internal/testing/containers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mailpitMessages struct {
	Total    int `json:"total"`
	Messages []struct {
		Subject string `json:"Subject"`
		To      []struct {
			Address string `json:"Address"`
		} `json:"To"`
	} `json:"messages"`
}

func fetchMessages(t *testing.T, apiBase string) mailpitMessages {
	t.Helper()
	var out mailpitMessages
	// Poll briefly; SMTP delivery is async relative to the HTTP API.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(apiBase + "/api/v1/messages")
		require.NoError(t, err)
		func() {
			defer func() { _ = resp.Body.Close() }()
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
		}()
		if out.Total > 0 {
			return out
		}
		time.Sleep(100 * time.Millisecond)
	}
	return out
}

func TestEmailSend_Engine(t *testing.T) {
	host, port, apiBase := containers.StartMailpit(t)

	svc, err := (&Plugin{}).CreateService(map[string]any{
		"host": host,
		"port": port,
		"from": "noda@test.local",
	})
	require.NoError(t, err)
	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("mailer", svc, nil))
	nodeReg := registry.NewNodeRegistry()
	require.NoError(t, nodeReg.RegisterFromPlugin(&Plugin{}))

	wf := engine.WorkflowConfig{
		ID: "email-send",
		Nodes: map[string]engine.NodeConfig{
			"m": {
				Type:     "email.send",
				Services: map[string]string{"mailer": "mailer"},
				Config: map[string]any{
					"to":      "recipient@example.com",
					"subject": "Noda E2E",
					"body":    "hello from noda",
				},
			},
		},
	}
	graph, err := engine.Compile(wf, nodeReg)
	require.NoError(t, err)
	execCtx := engine.NewExecutionContext(engine.WithInput(nil))
	require.NoError(t, engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg))

	msgs := fetchMessages(t, apiBase)
	require.Equal(t, 1, msgs.Total)
	assert.Equal(t, "Noda E2E", msgs.Messages[0].Subject)
	require.NotEmpty(t, msgs.Messages[0].To)
	assert.Equal(t, "recipient@example.com", msgs.Messages[0].To[0].Address)
}

func TestEmailSend_UnreachableHost_Engine(t *testing.T) {
	svc, err := (&Plugin{}).CreateService(map[string]any{
		"host": "127.0.0.1",
		"port": 1, // nothing listening
		"from": "noda@test.local",
	})
	require.NoError(t, err)
	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("mailer", svc, nil))
	nodeReg := registry.NewNodeRegistry()
	require.NoError(t, nodeReg.RegisterFromPlugin(&Plugin{}))

	wf := engine.WorkflowConfig{
		ID: "email-err",
		Nodes: map[string]engine.NodeConfig{
			"m": {
				Type:     "email.send",
				Services: map[string]string{"mailer": "mailer"},
				Config: map[string]any{
					"to":      "x@example.com",
					"subject": "fail",
					"body":    "fail",
				},
			},
		},
	}
	graph, err := engine.Compile(wf, nodeReg)
	require.NoError(t, err)
	execCtx := engine.NewExecutionContext(engine.WithInput(nil))
	err = engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.Error(t, err)
}
