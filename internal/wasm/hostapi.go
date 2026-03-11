package wasm

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/pkg/api"
)

// HostDispatcher handles noda_call and noda_call_async from Wasm modules.
type HostDispatcher struct {
	services *registry.ServiceRegistry
	runner   api.WorkflowRunner
	logger   *slog.Logger
	module   *Module // set after module creation
}

// NewHostDispatcher creates a new host dispatcher.
func NewHostDispatcher(services *registry.ServiceRegistry, runner api.WorkflowRunner, logger *slog.Logger) *HostDispatcher {
	return &HostDispatcher{
		services: services,
		runner:   runner,
		logger:   logger,
	}
}

// SetModule links the dispatcher to its module.
func (d *HostDispatcher) SetModule(m *Module) {
	d.module = m
}

// Call handles a synchronous noda_call request.
func (d *HostDispatcher) Call(ctx context.Context, req HostCallRequest) (any, error) {
	// Permission check
	if !d.module.IsServiceAllowed(req.Service) {
		return nil, fmt.Errorf("PERMISSION_DENIED: service %q not allowed", req.Service)
	}

	// System operations (service == "")
	if req.Service == "" {
		return d.handleSystemOp(ctx, req)
	}

	// Service dispatch
	svc, ok := d.services.Get(req.Service)
	if !ok {
		return nil, fmt.Errorf("SERVICE_UNAVAILABLE: service %q not found", req.Service)
	}

	return d.dispatchToService(ctx, svc, req)
}

// CallAsync handles an asynchronous noda_call_async request.
func (d *HostDispatcher) CallAsync(ctx context.Context, req HostCallRequest) error {
	if req.Label == "" {
		return fmt.Errorf("VALIDATION_ERROR: label is required for async calls")
	}

	// Register label
	if err := d.module.RegisterAsyncLabel(req.Label); err != nil {
		return fmt.Errorf("VALIDATION_ERROR: %s", err.Error())
	}

	// Permission check
	if !d.module.IsServiceAllowed(req.Service) {
		d.module.AddAsyncResult(req.Label, &AsyncResponse{
			Status: "error",
			Error: &AsyncError{
				Code:    "PERMISSION_DENIED",
				Message: fmt.Sprintf("service %q not allowed", req.Service),
			},
		})
		return nil
	}

	// Launch async operation
	label := req.Label
	go func() {
		result, err := d.Call(context.Background(), HostCallRequest{
			Service:   req.Service,
			Operation: req.Operation,
			Payload:   req.Payload,
		})

		if err != nil {
			d.module.AddAsyncResult(label, &AsyncResponse{
				Status: "error",
				Error: &AsyncError{
					Code:      "INTERNAL_ERROR",
					Message:   err.Error(),
					Operation: req.Service + "." + req.Operation,
				},
			})
			return
		}

		d.module.AddAsyncResult(label, &AsyncResponse{
			Status: "ok",
			Data:   result,
		})
	}()

	return nil
}

// handleSystemOp dispatches system-level operations.
func (d *HostDispatcher) handleSystemOp(ctx context.Context, req HostCallRequest) (any, error) {
	payload, _ := req.Payload.(map[string]any)
	if payload == nil {
		payload = make(map[string]any)
	}

	switch req.Operation {
	case "log":
		level, _ := payload["level"].(string)
		message, _ := payload["message"].(string)
		fields, _ := payload["fields"].(map[string]any)
		attrs := []any{"module", d.module.Name}
		for k, v := range fields {
			attrs = append(attrs, k, v)
		}
		switch level {
		case "debug":
			d.logger.Debug(message, attrs...)
		case "warn":
			d.logger.Warn(message, attrs...)
		case "error":
			d.logger.Error(message, attrs...)
		default:
			d.logger.Info(message, attrs...)
		}
		return nil, nil

	case "trigger_workflow":
		workflowID, _ := payload["workflow"].(string)
		if workflowID == "" {
			return nil, fmt.Errorf("VALIDATION_ERROR: workflow is required")
		}
		input, _ := payload["input"].(map[string]any)
		if d.runner != nil {
			go func() { _ = d.runner(context.Background(), workflowID, input) }()
		}
		return map[string]any{"status": "triggered"}, nil

	case "set_timer":
		name, _ := payload["name"].(string)
		if name == "" {
			return nil, fmt.Errorf("VALIDATION_ERROR: timer name is required")
		}
		intervalMs := int64(0)
		if v, ok := payload["interval"].(float64); ok {
			intervalMs = int64(v)
		}
		if intervalMs <= 0 {
			return nil, fmt.Errorf("VALIDATION_ERROR: interval must be positive")
		}
		d.module.SetTimer(name, intervalMs)
		return nil, nil

	case "clear_timer":
		name, _ := payload["name"].(string)
		if name == "" {
			return nil, fmt.Errorf("VALIDATION_ERROR: timer name is required")
		}
		d.module.ClearTimer(name)
		return nil, nil

	case "ws_connect":
		return d.module.gateway.Connect(ctx, payload)

	case "ws_send":
		return d.module.gateway.Send(payload)

	case "ws_close":
		return d.module.gateway.CloseConn(payload)

	case "ws_configure":
		return d.module.gateway.Configure(payload)

	default:
		return nil, fmt.Errorf("VALIDATION_ERROR: unknown system operation %q", req.Operation)
	}
}

// dispatchToService routes an operation to the appropriate service type.
func (d *HostDispatcher) dispatchToService(ctx context.Context, svc any, req HostCallRequest) (any, error) {
	payload, _ := req.Payload.(map[string]any)
	if payload == nil {
		payload = make(map[string]any)
	}

	switch s := svc.(type) {
	case api.CacheService:
		return d.dispatchCache(ctx, s, req.Operation, payload)
	case api.StorageService:
		return d.dispatchStorage(ctx, s, req.Operation, payload)
	case api.ConnectionService:
		return d.dispatchConnection(ctx, s, req.Operation, payload)
	case api.StreamService:
		return d.dispatchStream(ctx, s, req.Operation, payload)
	case api.PubSubService:
		return d.dispatchPubSub(ctx, s, req.Operation, payload)
	default:
		return nil, fmt.Errorf("SERVICE_UNAVAILABLE: unsupported service type for %q", req.Service)
	}
}

func (d *HostDispatcher) dispatchCache(ctx context.Context, svc api.CacheService, op string, payload map[string]any) (any, error) {
	switch op {
	case "get":
		key, _ := payload["key"].(string)
		val, err := svc.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		return map[string]any{"value": val}, nil
	case "set":
		key, _ := payload["key"].(string)
		value := payload["value"]
		ttl := 0
		if v, ok := payload["ttl"].(float64); ok {
			ttl = int(v)
		}
		return nil, svc.Set(ctx, key, value, ttl)
	case "del":
		key, _ := payload["key"].(string)
		return nil, svc.Del(ctx, key)
	case "exists":
		key, _ := payload["key"].(string)
		exists, err := svc.Exists(ctx, key)
		if err != nil {
			return nil, err
		}
		return map[string]any{"exists": exists}, nil
	default:
		return nil, fmt.Errorf("VALIDATION_ERROR: unknown cache operation %q", op)
	}
}

func (d *HostDispatcher) dispatchStorage(ctx context.Context, svc api.StorageService, op string, payload map[string]any) (any, error) {
	switch op {
	case "read":
		path, _ := payload["path"].(string)
		data, err := svc.Read(ctx, path)
		if err != nil {
			return nil, err
		}
		return map[string]any{"data": string(data)}, nil
	case "write":
		path, _ := payload["path"].(string)
		data, _ := payload["data"].(string)
		return nil, svc.Write(ctx, path, []byte(data))
	case "delete":
		path, _ := payload["path"].(string)
		return nil, svc.Delete(ctx, path)
	case "list":
		prefix, _ := payload["prefix"].(string)
		paths, err := svc.List(ctx, prefix)
		if err != nil {
			return nil, err
		}
		return map[string]any{"paths": paths}, nil
	default:
		return nil, fmt.Errorf("VALIDATION_ERROR: unknown storage operation %q", op)
	}
}

func (d *HostDispatcher) dispatchConnection(ctx context.Context, svc api.ConnectionService, op string, payload map[string]any) (any, error) {
	switch op {
	case "send":
		channel, _ := payload["channel"].(string)
		data := payload["data"]
		return nil, svc.Send(ctx, channel, data)
	case "send_sse":
		channel, _ := payload["channel"].(string)
		event, _ := payload["event"].(string)
		data := payload["data"]
		id, _ := payload["id"].(string)
		return nil, svc.SendSSE(ctx, channel, event, data, id)
	default:
		return nil, fmt.Errorf("VALIDATION_ERROR: unknown connection operation %q", op)
	}
}

func (d *HostDispatcher) dispatchStream(ctx context.Context, svc api.StreamService, op string, payload map[string]any) (any, error) {
	switch op {
	case "emit", "publish":
		topic, _ := payload["topic"].(string)
		data := payload["payload"]
		msgID, err := svc.Publish(ctx, topic, data)
		if err != nil {
			return nil, err
		}
		return map[string]any{"message_id": msgID}, nil
	default:
		return nil, fmt.Errorf("VALIDATION_ERROR: unknown stream operation %q", op)
	}
}

func (d *HostDispatcher) dispatchPubSub(ctx context.Context, svc api.PubSubService, op string, payload map[string]any) (any, error) {
	switch op {
	case "emit", "publish":
		channel, _ := payload["channel"].(string)
		data := payload["payload"]
		return nil, svc.Publish(ctx, channel, data)
	default:
		return nil, fmt.Errorf("VALIDATION_ERROR: unknown pubsub operation %q", op)
	}
}
