package server

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/gofiber/fiber/v3"
)

const defaultResponseTimeout = 30 * time.Second

// applyGlobalMiddleware applies global middleware from root config.
func (s *Server) applyGlobalMiddleware() error {
	globalMW := s.getGlobalMiddleware()
	for _, name := range globalMW {
		h, err := BuildMiddleware(name, s.config.Root)
		if err != nil {
			return fmt.Errorf("global middleware %q: %w", name, err)
		}
		s.app.Use(h)
	}
	return nil
}

// registerRoutes registers all route configs as Fiber handlers.
func (s *Server) registerRoutes() error {
	// Validate presets first
	if errs := s.ValidatePresets(); len(errs) > 0 {
		return errs[0]
	}

	for routeID, route := range s.config.Routes {
		if err := s.registerRoute(routeID, route); err != nil {
			return fmt.Errorf("route %q: %w", routeID, err)
		}
	}
	return nil
}

func (s *Server) registerRoute(routeID string, route map[string]any) error {
	method, _ := route["method"].(string)
	path, _ := route["path"].(string)

	if method == "" || path == "" {
		return fmt.Errorf("method and path are required")
	}

	// Resolve middleware chain for this route
	middlewareHandlers, err := s.ResolveMiddlewareChain(route)
	if err != nil {
		return err
	}

	// Get trigger config
	triggerConfig, _ := route["trigger"].(map[string]any)
	if triggerConfig == nil {
		return fmt.Errorf("trigger config is required")
	}
	workflowID, _ := triggerConfig["workflow"].(string)
	if workflowID == "" {
		return fmt.Errorf("trigger.workflow is required")
	}

	// Build handler
	handler := s.buildRouteHandler(routeID, workflowID, triggerConfig)

	// Apply route-level middleware using Use on a group, then register handler
	// Fiber v3: app.Get(path, handler, ...middleware) uses `any` type
	// We use middleware as Use() on the app before route, or chain them inline
	for _, mw := range middlewareHandlers {
		s.app.Use(path, mw)
	}

	// Register route by method
	switch strings.ToUpper(method) {
	case "GET":
		s.app.Get(path, handler)
	case "POST":
		s.app.Post(path, handler)
	case "PUT":
		s.app.Put(path, handler)
	case "PATCH":
		s.app.Patch(path, handler)
	case "DELETE":
		s.app.Delete(path, handler)
	default:
		return fmt.Errorf("unsupported HTTP method: %s", method)
	}

	s.logger.Debug("route registered", "id", routeID, "method", method, "path", path)
	return nil
}

// buildRouteHandler creates the Fiber handler that runs trigger mapping → workflow → response.
func (s *Server) buildRouteHandler(routeID, workflowID string, triggerConfig map[string]any) fiber.Handler {
	return func(c fiber.Ctx) error {
		// 1. Trigger mapping
		triggerResult, err := MapTrigger(c, triggerConfig, s.compiler)
		if err != nil {
			s.logger.Error("trigger mapping failed", "route", routeID, "error", err)
			return writeErrorResponse(c, 400, ErrorResponse{
				Error: api.ErrorData{
					Code:    "TRIGGER_MAPPING_ERROR",
					Message: err.Error(),
				},
			})
		}

		// 2. Build execution context
		execCtx := engine.NewExecutionContext(
			engine.WithInput(triggerResult.Input),
			engine.WithAuth(triggerResult.Auth),
			engine.WithTrigger(triggerResult.Trigger),
			engine.WithWorkflowID(workflowID),
			engine.WithCompiler(s.compiler),
		)

		// 3. Create response channel
		responseCh := make(chan *api.HTTPResponse, 1)

		// 4. Register response interceptor on the execution context
		execCtx.SetResponseInterceptor(func(resp *api.HTTPResponse) {
			select {
			case responseCh <- resp:
			default:
				// Already sent a response — ignore subsequent ones
			}
		})

		// 5. Start workflow in goroutine
		workflowDone := make(chan error, 1)
		go func() {
			workflowDone <- s.runWorkflow(c.Context(), workflowID, execCtx)
		}()

		// 6. Wait for response or workflow completion or timeout
		responseTimeout := defaultResponseTimeout
		if serverCfg, ok := s.config.Root["server"].(map[string]any); ok {
			if v, ok := serverCfg["response_timeout"].(string); ok {
				if d, err := time.ParseDuration(v); err == nil {
					responseTimeout = d
				}
			}
		}

		timer := time.NewTimer(responseTimeout)
		defer timer.Stop()

		select {
		case resp := <-responseCh:
			// Response node fired — send response immediately
			return writeHTTPResponse(c, resp)

		case wfErr := <-workflowDone:
			// Workflow completed without sending a response
			if wfErr != nil {
				status, errResp := MapErrorToHTTP(wfErr, triggerResult.Trigger.TraceID)
				return writeErrorResponse(c, status, errResp)
			}
			// No response node → 202 Accepted
			return c.Status(fiber.StatusAccepted).JSON(map[string]any{
				"status":   "accepted",
				"trace_id": triggerResult.Trigger.TraceID,
			})

		case <-timer.C:
			// Response timeout
			return writeErrorResponse(c, 504, ErrorResponse{
				Error: api.ErrorData{
					Code:    "TIMEOUT",
					Message: "Response timeout exceeded",
					TraceID: triggerResult.Trigger.TraceID,
				},
			})
		}
	}
}

// runWorkflow compiles and executes a workflow.
func (s *Server) runWorkflow(ctx context.Context, workflowID string, execCtx *engine.ExecutionContextImpl) error {
	wfData, ok := s.config.Workflows[workflowID]
	if !ok {
		return fmt.Errorf("workflow %q not found", workflowID)
	}

	wfConfig, err := engine.ParseWorkflowFromMap(workflowID, wfData)
	if err != nil {
		return fmt.Errorf("parse workflow %q: %w", workflowID, err)
	}

	graph, err := engine.Compile(wfConfig, s.nodes)
	if err != nil {
		return fmt.Errorf("compile workflow %q: %w", workflowID, err)
	}

	return engine.ExecuteGraph(ctx, graph, execCtx, s.services, s.nodes)
}
