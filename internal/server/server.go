package server

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/gofiber/fiber/v3"
)

// Server wraps the Fiber app and Noda runtime dependencies.
type Server struct {
	app      *fiber.App
	config   *config.ResolvedConfig
	compiler *expr.Compiler
	services *registry.ServiceRegistry
	nodes    *registry.NodeRegistry
	port     int
	logger   *slog.Logger
}

// ServerOption configures a Server.
type ServerOption func(*Server)

// WithLogger sets the server logger.
func WithLogger(logger *slog.Logger) ServerOption {
	return func(s *Server) { s.logger = logger }
}

// NewServer creates a Fiber app from the resolved config.
func NewServer(rc *config.ResolvedConfig, services *registry.ServiceRegistry, nodes *registry.NodeRegistry, opts ...ServerOption) (*Server, error) {
	s := &Server{
		config:   rc,
		compiler: expr.NewCompilerWithFunctions(),
		services: services,
		nodes:    nodes,
		port:     3000,
		logger:   slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}

	// Read server settings from root config
	if serverCfg, ok := rc.Root["server"].(map[string]any); ok {
		if p, ok := serverCfg["port"].(float64); ok {
			s.port = int(p)
		}
	}

	fiberCfg := fiber.Config{
		ErrorHandler: s.errorHandler,
	}

	// Apply timeouts and body limit from config
	if serverCfg, ok := rc.Root["server"].(map[string]any); ok {
		if v, ok := serverCfg["read_timeout"].(string); ok {
			if d, err := time.ParseDuration(v); err == nil {
				fiberCfg.ReadTimeout = d
			}
		}
		if v, ok := serverCfg["write_timeout"].(string); ok {
			if d, err := time.ParseDuration(v); err == nil {
				fiberCfg.WriteTimeout = d
			}
		}
		if v, ok := serverCfg["body_limit"].(float64); ok {
			fiberCfg.BodyLimit = int(v)
		}
	}

	s.app = fiber.New(fiberCfg)
	return s, nil
}

// App returns the underlying Fiber app (for testing).
func (s *Server) App() *fiber.App { return s.app }

// Port returns the configured port.
func (s *Server) Port() int { return s.port }

// Setup registers middleware and routes on the Fiber app.
func (s *Server) Setup() error {
	// Apply global middleware
	if err := s.applyGlobalMiddleware(); err != nil {
		return fmt.Errorf("global middleware: %w", err)
	}

	// Register routes
	if err := s.registerRoutes(); err != nil {
		return fmt.Errorf("register routes: %w", err)
	}

	return nil
}

// Start begins listening on the configured port.
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.port)
	s.logger.Info("server starting", "addr", addr)
	return s.app.Listen(addr)
}

// Stop gracefully shuts down the server.
func (s *Server) Stop() error {
	s.logger.Info("server stopping")
	return s.app.Shutdown()
}

// errorHandler is the Fiber error handler that returns standardized error responses.
func (s *Server) errorHandler(c fiber.Ctx, err error) error {
	status := fiber.StatusInternalServerError
	code := "INTERNAL_ERROR"
	message := "Internal server error"

	if fe, ok := err.(*fiber.Error); ok {
		status = fe.Code
		message = fe.Message
		switch status {
		case fiber.StatusNotFound:
			code = "NOT_FOUND"
		case fiber.StatusMethodNotAllowed:
			code = "METHOD_NOT_ALLOWED"
		case fiber.StatusTooManyRequests:
			code = "RATE_LIMITED"
		case fiber.StatusUnauthorized:
			code = "UNAUTHORIZED"
		case fiber.StatusForbidden:
			code = "FORBIDDEN"
		case fiber.StatusRequestTimeout:
			code = "TIMEOUT"
		default:
			code = "HTTP_ERROR"
		}
	}

	return c.Status(status).JSON(map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}

// executeWorkflow compiles and runs a workflow, returning the execution context.
func (s *Server) executeWorkflow(ctx fiber.Ctx, workflowID string, execCtx *engine.ExecutionContextImpl) error {
	wfData, ok := s.config.Workflows[workflowID]
	if !ok {
		return fmt.Errorf("workflow %q not found", workflowID)
	}

	wfConfig, err := parseWorkflowConfig(workflowID, wfData)
	if err != nil {
		return fmt.Errorf("parse workflow %q: %w", workflowID, err)
	}

	graph, err := engine.Compile(wfConfig, s.nodes)
	if err != nil {
		return fmt.Errorf("compile workflow %q: %w", workflowID, err)
	}

	return engine.ExecuteGraph(ctx.Context(), graph, execCtx, s.services, s.nodes)
}

// parseWorkflowConfig converts a raw workflow map into an engine.WorkflowConfig.
func parseWorkflowConfig(id string, raw map[string]any) (engine.WorkflowConfig, error) {
	wf := engine.WorkflowConfig{
		ID:    id,
		Nodes: make(map[string]engine.NodeConfig),
	}

	nodesRaw, _ := raw["nodes"].(map[string]any)
	for nodeID, nodeRaw := range nodesRaw {
		nm, ok := nodeRaw.(map[string]any)
		if !ok {
			return wf, fmt.Errorf("node %q: invalid format", nodeID)
		}
		nc := engine.NodeConfig{
			Type: mapStrVal(nm, "type"),
			As:   mapStrVal(nm, "as"),
		}
		if cfg, ok := nm["config"].(map[string]any); ok {
			nc.Config = cfg
		}
		if svc, ok := nm["services"].(map[string]any); ok {
			nc.Services = make(map[string]string)
			for k, v := range svc {
				nc.Services[k] = fmt.Sprintf("%v", v)
			}
		}
		wf.Nodes[nodeID] = nc
	}

	edgesRaw, _ := raw["edges"].([]any)
	for _, edgeRaw := range edgesRaw {
		em, ok := edgeRaw.(map[string]any)
		if !ok {
			continue
		}
		ec := engine.EdgeConfig{
			From:   mapStrVal(em, "from"),
			To:     mapStrVal(em, "to"),
			Output: mapStrVal(em, "output"),
		}
		if retryRaw, ok := em["retry"].(map[string]any); ok {
			ec.Retry = &engine.RetryConfig{
				Backoff: mapStrVal(retryRaw, "backoff"),
				Delay:   mapStrVal(retryRaw, "delay"),
			}
			if a, ok := retryRaw["attempts"].(float64); ok {
				ec.Retry.Attempts = int(a)
			}
		}
		wf.Edges = append(wf.Edges, ec)
	}

	return wf, nil
}

func mapStrVal(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}
