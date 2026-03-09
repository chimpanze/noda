package server

import (
	"context"
	"time"

	"github.com/chimpanze/noda/internal/connmgr"
	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/google/uuid"
)

// registerConnections sets up WebSocket and SSE endpoints from connection config.
func (s *Server) registerConnections() error {
	for _, connConfig := range s.config.Connections {
		endpoints, ok := connConfig["endpoints"].(map[string]any)
		if !ok {
			continue
		}

		for name, endpointRaw := range endpoints {
			ep, ok := endpointRaw.(map[string]any)
			if !ok {
				continue
			}

			epType, _ := ep["type"].(string)
			path, _ := ep["path"].(string)
			if path == "" {
				continue
			}

			// Create a manager per endpoint
			mgr := connmgr.NewManager()
			svc := connmgr.NewEndpointService(mgr, name)

			// Register as a service so workflow nodes can reference it
			prefix := epType // "websocket" → register under "ws", "sse" → "sse"
			if prefix == "websocket" {
				prefix = "ws"
			}
			s.services.Register(name, svc, nil)

			// Build workflow runner
			runner := s.buildWorkflowRunner()

			// Extract channel pattern
			channelPattern := ""
			if channels, ok := ep["channels"].(map[string]any); ok {
				channelPattern, _ = channels["pattern"].(string)
			}

			switch epType {
			case "websocket":
				cfg := connmgr.WebSocketConfig{
					Endpoint:       name,
					Path:           path,
					ChannelPattern: channelPattern,
					OnConnect:      mapStr(ep, "on_connect"),
					OnMessage:      mapStr(ep, "on_message"),
					OnDisconnect:   mapStr(ep, "on_disconnect"),
				}

				if v, _ := ep["ping_interval"].(string); v != "" {
					if d, err := time.ParseDuration(v); err == nil {
						cfg.PingInterval = d
					}
				}

				if channels, ok := ep["channels"].(map[string]any); ok {
					if v, ok := channels["max_per_channel"].(float64); ok {
						cfg.MaxPerChannel = int(v)
					}
				}

				handler := connmgr.NewWebSocketHandler(cfg, mgr, runner, s.logger)
				handler.Register(s.app)
				s.logger.Debug("websocket endpoint registered", "name", name, "path", path)

			case "sse":
				cfg := connmgr.SSEConfig{
					Endpoint:       name,
					Path:           path,
					ChannelPattern: channelPattern,
					OnConnect:      mapStr(ep, "on_connect"),
					OnDisconnect:   mapStr(ep, "on_disconnect"),
				}

				if v, _ := ep["heartbeat"].(string); v != "" {
					if d, err := time.ParseDuration(v); err == nil {
						cfg.Heartbeat = d
					}
				}
				if v, ok := ep["retry"].(float64); ok {
					cfg.Retry = int(v)
				}

				handler := connmgr.NewSSEHandler(cfg, mgr, runner, s.logger)
				handler.Register(s.app)
				s.logger.Debug("sse endpoint registered", "name", name, "path", path)
			}
		}
	}
	return nil
}

// buildWorkflowRunner creates a WorkflowRunner that uses the server's engine.
func (s *Server) buildWorkflowRunner() connmgr.WorkflowRunner {
	return func(ctx context.Context, workflowID string, input map[string]any) error {
		execCtx := engine.NewExecutionContext(
			engine.WithInput(input),
			engine.WithTrigger(api.TriggerData{
				Type:      "websocket",
				TraceID:   uuid.New().String(),
			}),
			engine.WithWorkflowID(workflowID),
			engine.WithCompiler(expr.NewCompilerWithFunctions()),
		)
		return s.runWorkflow(ctx, workflowID, execCtx)
	}
}

func mapStr(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}
