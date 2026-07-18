package server

import (
	"context"
	"fmt"
	"time"

	"github.com/chimpanze/noda/internal/connmgr"
	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

// registerConnections sets up WebSocket and SSE endpoints from connection config.
func (s *Server) registerConnections() error {
	for _, connConfig := range s.config.Connections {
		// Build the sync bridge once per config file (not per endpoint): every
		// endpoint in this file shares the same "sync" block, and a broken
		// sync config should boot-error even if the file declares zero
		// endpoints.
		var bridge *connmgr.SyncBridge
		if syncCfg, ok := connConfig["sync"].(map[string]any); ok {
			pubsubName, _ := syncCfg["pubsub"].(string)
			if pubsubName != "" {
				raw, found := s.services.Get(pubsubName)
				if !found {
					return fmt.Errorf("connections sync: pubsub service %q not found", pubsubName)
				}
				ps, ok := raw.(api.PubSubService)
				if !ok {
					return fmt.Errorf("connections sync: service %q does not implement PubSubService", pubsubName)
				}
				bridge = connmgr.NewSyncBridge(ps, s.instanceID, s.logger)
			}
		}

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

			// Create a manager per endpoint with optional limits
			mgrCfg := connmgr.ManagerConfig{}
			if v, ok := ep["max_connections"].(float64); ok && v > 0 {
				mgrCfg.MaxTotalConnections = int(v)
			}
			if channels, ok := ep["channels"].(map[string]any); ok {
				if v, ok := channels["max_per_channel"].(float64); ok && v > 0 {
					mgrCfg.MaxConnectionsPerChannel = int(v)
				}
			}
			mgr := connmgr.NewManager(mgrCfg)
			s.connManagers.Add(mgr)

			svc := connmgr.NewEndpointService(mgr, name, bridge)

			// Register as a service so workflow nodes can reference it
			if err := s.services.Register(name, svc, nil); err != nil {
				s.logger.Warn("connection service registration failed", "name", name, "error", err)
			}

			// Extract channel pattern
			channelPattern := ""
			if channels, ok := ep["channels"].(map[string]any); ok {
				channelPattern, _ = channels["pattern"].(string)
			}

			// Resolve middleware for this endpoint (auth, etc.)
			middleware, err := s.resolveEndpointMiddleware(ep)
			if err != nil {
				return fmt.Errorf("endpoint %q middleware: %w", name, err)
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

				// Validate lifecycle workflow references at startup
				if err := s.validateWorkflowRefs(name, cfg.OnConnect, cfg.OnMessage, cfg.OnDisconnect); err != nil {
					return err
				}

				runner := s.buildWorkflowRunner("websocket")
				handler := connmgr.NewWebSocketHandler(cfg, mgr, runner, s.compiler, s.logger)
				handler.Register(s.app, middleware...)
				s.logger.Info("connection endpoint registered", "name", name, "type", "websocket", "path", path)

				if bridge != nil {
					go bridge.Run(s.syncCtx, name, mgr)
					s.logger.Info("cross-instance sync active", "endpoint", name)
				}

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

				// Validate lifecycle workflow references at startup
				if err := s.validateWorkflowRefs(name, cfg.OnConnect, cfg.OnDisconnect); err != nil {
					return err
				}

				runner := s.buildWorkflowRunner("sse")
				handler := connmgr.NewSSEHandler(cfg, mgr, runner, s.compiler, s.logger)
				handler.Register(s.app, middleware...)
				s.logger.Info("connection endpoint registered", "name", name, "type", "sse", "path", path)

				if bridge != nil {
					go bridge.Run(s.syncCtx, name, mgr)
					s.logger.Info("cross-instance sync active", "endpoint", name)
				}
			}
		}
	}
	return nil
}

// validateWorkflowRefs checks that all non-empty workflow IDs exist in the cache.
func (s *Server) validateWorkflowRefs(endpoint string, workflowIDs ...string) error {
	for _, id := range workflowIDs {
		if id == "" {
			continue
		}
		if _, ok := s.workflows.Get(id); !ok {
			return fmt.Errorf("endpoint %q: workflow %q not found", endpoint, id)
		}
	}
	return nil
}

// buildWorkflowRunner creates a WorkflowRunner that uses the server's engine.
func (s *Server) buildWorkflowRunner(triggerType string) api.WorkflowRunner {
	return func(ctx context.Context, workflowID string, input map[string]any) error {
		opts := []engine.ExecutionContextOption{
			engine.WithInput(input),
			engine.WithTrigger(api.TriggerData{
				Type:    triggerType,
				TraceID: uuid.New().String(),
			}),
			engine.WithWorkflowID(workflowID),
			engine.WithCompiler(s.compiler),
			engine.WithSecrets(s.secretsContext),
		}
		if s.subWorkflowRunner != nil {
			opts = append(opts, engine.WithSubWorkflowRunner(s.subWorkflowRunner))
		}
		execCtx := engine.NewExecutionContext(opts...)
		return s.runWorkflow(ctx, workflowID, execCtx)
	}
}

// resolveEndpointMiddlewareNames resolves the ordered, deduplicated middleware
// names for a connection endpoint and validates ordering constraints, without
// building any handlers.
func (s *Server) resolveEndpointMiddlewareNames(ep map[string]any) ([]string, error) {
	var middlewareNames []string

	// Expand preset if specified
	if preset, ok := ep["middleware_preset"].(string); ok && preset != "" {
		expanded, err := s.expandPreset(preset)
		if err != nil {
			return nil, err
		}
		middlewareNames = append(middlewareNames, expanded...)
	}

	// Direct middleware list
	if mwList, ok := ep["middleware"].([]any); ok {
		for _, mw := range mwList {
			if name, ok := mw.(string); ok {
				middlewareNames = append(middlewareNames, name)
			}
		}
	}

	middlewareNames = dedupe(middlewareNames)

	if err := ValidateMiddlewareOrder(middlewareNames); err != nil {
		return nil, err
	}

	return middlewareNames, nil
}

// resolveEndpointMiddleware resolves middleware handlers for a connection endpoint.
// Supports both "middleware": ["auth.jwt"] and "middleware_preset": "authenticated".
func (s *Server) resolveEndpointMiddleware(ep map[string]any) ([]fiber.Handler, error) {
	middlewareNames, err := s.resolveEndpointMiddlewareNames(ep)
	if err != nil {
		return nil, err
	}

	handlers := make([]fiber.Handler, 0, len(middlewareNames))
	for _, name := range middlewareNames {
		h, err := s.buildMiddleware(name)
		if err != nil {
			return nil, fmt.Errorf("middleware %q: %w", name, err)
		}
		handlers = append(handlers, h)
	}

	return handlers, nil
}

func mapStr(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}
