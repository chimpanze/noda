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
	app       *fiber.App
	config    *config.ResolvedConfig
	compiler  *expr.Compiler
	services  *registry.ServiceRegistry
	nodes     *registry.NodeRegistry
	workflows *engine.WorkflowCache
	port      int
	logger    *slog.Logger
}

// ServerOption configures a Server.
type ServerOption func(*Server)

// WithLogger sets the server logger.
func WithLogger(logger *slog.Logger) ServerOption {
	return func(s *Server) { s.logger = logger }
}

// WithCompiler sets a shared expression compiler.
func WithCompiler(c *expr.Compiler) ServerOption {
	return func(s *Server) { s.compiler = c }
}

// WithWorkflowCache sets a pre-built workflow cache.
func WithWorkflowCache(c *engine.WorkflowCache) ServerOption {
	return func(s *Server) { s.workflows = c }
}

// NewServer creates a Fiber app from the resolved config.
func NewServer(rc *config.ResolvedConfig, services *registry.ServiceRegistry, nodes *registry.NodeRegistry, opts ...ServerOption) (*Server, error) {
	s := &Server{
		config:   rc,
		services: services,
		nodes:    nodes,
		port:     3000,
		logger:   slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.compiler == nil {
		s.compiler = expr.NewCompilerWithFunctions()
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
	// Pre-compile all workflows (skip if cache was provided via WithWorkflowCache)
	if s.workflows == nil {
		cache, err := engine.NewWorkflowCache(s.config.Workflows, s.nodes)
		if err != nil {
			return fmt.Errorf("compile workflows: %w", err)
		}
		s.workflows = cache
	}

	// Register health endpoints (before middleware so they're always accessible)
	s.registerHealthRoutes()

	// Apply global middleware
	if err := s.applyGlobalMiddleware(); err != nil {
		return fmt.Errorf("global middleware: %w", err)
	}

	// Register routes
	if err := s.registerRoutes(); err != nil {
		return fmt.Errorf("register routes: %w", err)
	}

	// Register WebSocket and SSE endpoints
	if err := s.registerConnections(); err != nil {
		return fmt.Errorf("register connections: %w", err)
	}

	return nil
}

// WorkflowCache returns the server's workflow cache (for worker/scheduler sharing).
func (s *Server) WorkflowCache() *engine.WorkflowCache { return s.workflows }

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
