package engine

import (
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/google/uuid"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// exprContextPool reuses the outer 4-key map in buildExprContext to reduce allocations.
var exprContextPool = sync.Pool{
	New: func() any {
		return make(map[string]any, 4)
	},
}

// ResponseInterceptor is called when a node produces an HTTPResponse.
type ResponseInterceptor func(resp *api.HTTPResponse)

// ExecutionContextImpl implements api.ExecutionContext for workflow execution.
type ExecutionContextImpl struct {
	input   any
	auth    *api.AuthData
	trigger api.TriggerData

	mu      sync.RWMutex
	outputs map[string]any    // nodeID (or alias) → output data
	aliases map[string]string // nodeID → alias (from "as" field)

	compiler *expr.Compiler
	logger   *slog.Logger

	workflowID          string
	currentNode         atomic.Value // set during node execution
	responseInterceptor ResponseInterceptor
	tracer              oteltrace.Tracer
	traceCallback       func(eventType, nodeID, nodeType, output, errMsg string, data any)

	depth    int32 // atomic
	maxDepth int32 // atomic
}

// NewExecutionContext creates a new execution context for a workflow run.
func NewExecutionContext(opts ...ExecutionContextOption) *ExecutionContextImpl {
	ctx := &ExecutionContextImpl{
		outputs: make(map[string]any),
		aliases: make(map[string]string),
		trigger: api.TriggerData{
			TraceID: uuid.New().String(),
		},
		// Fallback compiler for tests. Production paths inject the shared compiler via WithCompiler().
		compiler: expr.NewCompilerWithFunctions(),
		logger:   slog.Default(),
		tracer:   noop.NewTracerProvider().Tracer("noda"),
	}
	atomic.StoreInt32(&ctx.maxDepth, 64)
	for _, opt := range opts {
		opt(ctx)
	}
	return ctx
}

// ExecutionContextOption configures an ExecutionContext.
type ExecutionContextOption func(*ExecutionContextImpl)

// WithInput sets the input data.
func WithInput(input any) ExecutionContextOption {
	return func(c *ExecutionContextImpl) { c.input = input }
}

// WithAuth sets the auth data.
func WithAuth(auth *api.AuthData) ExecutionContextOption {
	return func(c *ExecutionContextImpl) { c.auth = auth }
}

// WithTrigger sets the trigger data.
func WithTrigger(trigger api.TriggerData) ExecutionContextOption {
	return func(c *ExecutionContextImpl) {
		if trigger.TraceID == "" {
			trigger.TraceID = uuid.New().String()
		}
		c.trigger = trigger
	}
}

// WithWorkflowID sets the workflow ID for logging.
func WithWorkflowID(id string) ExecutionContextOption {
	return func(c *ExecutionContextImpl) { c.workflowID = id }
}

// WithLogger sets the logger.
func WithLogger(logger *slog.Logger) ExecutionContextOption {
	return func(c *ExecutionContextImpl) { c.logger = logger }
}

// WithCompiler sets the expression compiler.
func WithCompiler(compiler *expr.Compiler) ExecutionContextOption {
	return func(c *ExecutionContextImpl) { c.compiler = compiler }
}

// WithTracer sets the OTel tracer.
func WithTracer(tracer oteltrace.Tracer) ExecutionContextOption {
	return func(c *ExecutionContextImpl) { c.tracer = tracer }
}

// Tracer returns the OTel tracer.
func (c *ExecutionContextImpl) Tracer() oteltrace.Tracer { return c.tracer }

// TraceCallback is a function called for each execution event (dev mode).
type TraceCallback func(eventType, nodeID, nodeType, output, errMsg string, data any)

// WithTraceCallback sets a callback for dev-mode trace events.
func WithTraceCallback(fn TraceCallback) ExecutionContextOption {
	return func(c *ExecutionContextImpl) { c.traceCallback = fn }
}

// EmitTrace sends a trace event if a callback is configured.
func (c *ExecutionContextImpl) EmitTrace(eventType, nodeID, nodeType, output, errMsg string, data any) {
	if c.traceCallback != nil {
		c.traceCallback(eventType, nodeID, nodeType, output, errMsg, data)
	}
}

// Input returns the workflow input data.
func (c *ExecutionContextImpl) Input() any { return c.input }

// Auth returns the auth data, or nil if not authenticated.
func (c *ExecutionContextImpl) Auth() *api.AuthData { return c.auth }

// Trigger returns the trigger data including trace ID.
func (c *ExecutionContextImpl) Trigger() api.TriggerData { return c.trigger }

// Resolve evaluates an expression against the current context including node outputs.
func (c *ExecutionContextImpl) Resolve(expression string) (any, error) {
	c.mu.RLock()
	context := c.buildExprContext()
	c.mu.RUnlock()
	defer returnExprContext(context)

	resolver := expr.NewResolver(c.compiler, context)
	return resolver.Resolve(expression)
}

// ResolveWithVars evaluates an expression with additional variables in scope.
// Extra vars are overlaid on top of the standard context (input, auth, node outputs).
func (c *ExecutionContextImpl) ResolveWithVars(expression string, extraVars map[string]any) (any, error) {
	c.mu.RLock()
	context := c.buildExprContext()
	c.mu.RUnlock()
	defer returnExprContext(context)

	for k, v := range extraVars {
		context[k] = v
	}

	resolver := expr.NewResolver(c.compiler, context)
	return resolver.Resolve(expression)
}

// Log writes a structured log entry with trace context.
func (c *ExecutionContextImpl) Log(level string, message string, fields map[string]any) {
	attrs := []any{
		"trace_id", c.trigger.TraceID,
		"workflow_id", c.workflowID,
	}
	if nodeID, _ := c.currentNode.Load().(string); nodeID != "" {
		attrs = append(attrs, "node_id", nodeID)
	}
	for k, v := range fields {
		attrs = append(attrs, k, v)
	}

	switch level {
	case "debug":
		c.logger.Debug(message, attrs...)
	case "warn":
		c.logger.Warn(message, attrs...)
	case "error":
		c.logger.Error(message, attrs...)
	default:
		c.logger.Info(message, attrs...)
	}
}

// SetOutput stores a node's output data.
func (c *ExecutionContextImpl) SetOutput(nodeID string, data any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := nodeID
	if alias, ok := c.aliases[nodeID]; ok {
		key = alias
	}
	c.outputs[key] = data
}

// GetOutput retrieves a node's output data.
func (c *ExecutionContextImpl) GetOutput(nodeID string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Check alias first
	if alias, ok := c.aliases[nodeID]; ok {
		v, found := c.outputs[alias]
		return v, found
	}
	v, ok := c.outputs[nodeID]
	return v, ok
}

// RegisterAlias registers an "as" alias for a node.
func (c *ExecutionContextImpl) RegisterAlias(nodeID, alias string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.aliases[nodeID] = alias
}

// SetCurrentNode sets the current node ID for logging context.
func (c *ExecutionContextImpl) SetCurrentNode(nodeID string) {
	c.currentNode.Store(nodeID)
}

// SetResponseInterceptor sets a callback for HTTPResponse interception.
func (c *ExecutionContextImpl) SetResponseInterceptor(fn ResponseInterceptor) {
	c.responseInterceptor = fn
}

// InterceptResponse checks if data is an HTTPResponse and notifies the interceptor.
func (c *ExecutionContextImpl) InterceptResponse(data any) {
	if c.responseInterceptor == nil {
		return
	}
	if resp, ok := data.(*api.HTTPResponse); ok {
		c.responseInterceptor(resp)
	}
}

// EvictOutput removes an output from the context.
func (c *ExecutionContextImpl) EvictOutput(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.outputs, key)
}

// CheckAndIncrementDepth checks if the recursion depth limit has been reached.
// If not, it increments the depth counter. Returns an error if the limit is exceeded.
func (c *ExecutionContextImpl) CheckAndIncrementDepth() error {
	newDepth := atomic.AddInt32(&c.depth, 1)
	max := atomic.LoadInt32(&c.maxDepth)
	if newDepth > max {
		atomic.AddInt32(&c.depth, -1)
		return fmt.Errorf("maximum recursion depth (%d) exceeded", max)
	}
	return nil
}

// DecrementDepth decrements the recursion depth counter.
func (c *ExecutionContextImpl) DecrementDepth() {
	atomic.AddInt32(&c.depth, -1)
}

// buildExprContext creates the expression evaluation context map.
// The returned map is from a pool — callers must call returnExprContext when done.
func (c *ExecutionContextImpl) buildExprContext() map[string]any {
	ctx := exprContextPool.Get().(map[string]any)
	// Clear stale keys
	for k := range ctx {
		delete(ctx, k)
	}
	ctx["input"] = c.input
	if c.auth != nil {
		ctx["auth"] = map[string]any{
			"sub":    c.auth.UserID,
			"roles":  c.auth.Roles,
			"claims": c.auth.Claims,
		}
	}
	ctx["trigger"] = map[string]any{
		"type":      c.trigger.Type,
		"timestamp": c.trigger.Timestamp,
		"trace_id":  c.trigger.TraceID,
	}
	// Node outputs are namespaced under "nodes" to avoid clashing with
	// expr-lang built-in functions (len, find, count, map, filter, etc.).
	nodesMap := make(map[string]any, len(c.outputs))
	for k, v := range c.outputs {
		nodesMap[k] = v
	}
	ctx["nodes"] = nodesMap
	return ctx
}

func returnExprContext(ctx map[string]any) {
	exprContextPool.Put(ctx)
}
