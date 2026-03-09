package server

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
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

	// Validate workflow exists at startup (fail at load time, not runtime)
	if _, ok := s.workflows.Get(workflowID); !ok {
		return fmt.Errorf("trigger.workflow %q not found in workflow cache", workflowID)
	}

	// Build handler with route-level middleware composed inline.
	// This ensures middleware only applies to this specific method+path,
	// not to all methods on the same path.
	routeHandler := s.buildRouteHandler(routeID, workflowID, triggerConfig)

	// Build handler chain: middleware first, then the route handler.
	// Fiber v3 executes handlers in registration order, calling c.Next() to advance.
	allHandlers := make([]any, 0, len(middlewareHandlers)+1)
	for _, mw := range middlewareHandlers {
		allHandlers = append(allHandlers, mw)
	}
	allHandlers = append(allHandlers, routeHandler)

	// Register route by method
	switch strings.ToUpper(method) {
	case "GET":
		s.app.Get(path, allHandlers[0], allHandlers[1:]...)
	case "POST":
		s.app.Post(path, allHandlers[0], allHandlers[1:]...)
	case "PUT":
		s.app.Put(path, allHandlers[0], allHandlers[1:]...)
	case "PATCH":
		s.app.Patch(path, allHandlers[0], allHandlers[1:]...)
	case "DELETE":
		s.app.Delete(path, allHandlers[0], allHandlers[1:]...)
	default:
		return fmt.Errorf("unsupported HTTP method: %s", method)
	}

	s.logger.Debug("route registered", "id", routeID, "method", method, "path", path)
	return nil
}

// buildRouteHandler creates the Fiber handler that runs trigger mapping → workflow → response.
func (s *Server) buildRouteHandler(routeID, workflowID string, triggerConfig map[string]any) fiber.Handler {
	return func(c fiber.Ctx) error {
		// Generate trace ID early so it's available for all error paths
		traceID := uuid.New().String()

		// 1. Trigger mapping
		triggerResult, err := MapTrigger(c, triggerConfig, s.compiler)
		if err != nil {
			s.logger.Error("trigger mapping failed", "route", routeID, "error", err, "trace_id", traceID)
			return writeErrorResponse(c, 400, ErrorResponse{
				Error: api.ErrorData{
					Code:    "TRIGGER_MAPPING_ERROR",
					Message: err.Error(),
					TraceID: traceID,
				},
			})
		}
		triggerResult.Trigger.TraceID = traceID

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

		// 5. Start workflow in goroutine with cancellable context so it
		// is cleaned up on response timeout or early handler return.
		ctx, cancel := context.WithCancel(c.Context())
		defer cancel()

		workflowDone := make(chan error, 1)
		go func() {
			workflowDone <- s.runWorkflow(ctx, workflowID, execCtx)
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
				status, errResp := MapErrorToHTTP(wfErr, traceID)
				return writeErrorResponse(c, status, errResp)
			}
			// No response node → 202 Accepted
			return c.Status(fiber.StatusAccepted).JSON(map[string]any{
				"status":   "accepted",
				"trace_id": traceID,
			})

		case <-timer.C:
			// Response timeout — cancel cancels the workflow goroutine
			return writeErrorResponse(c, 504, ErrorResponse{
				Error: api.ErrorData{
					Code:    "TIMEOUT",
					Message: "Response timeout exceeded",
					TraceID: traceID,
				},
			})
		}
	}
}

// runWorkflow executes a pre-compiled workflow from the cache.
func (s *Server) runWorkflow(ctx context.Context, workflowID string, execCtx *engine.ExecutionContextImpl) error {
	graph, ok := s.workflows.Get(workflowID)
	if !ok {
		return fmt.Errorf("workflow %q not found", workflowID)
	}

	return engine.ExecuteGraph(ctx, graph, execCtx, s.services, s.nodes)
}
