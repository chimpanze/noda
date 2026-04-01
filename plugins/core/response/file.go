package response

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
)

type fileDescriptor struct{}

func (d *fileDescriptor) Name() string        { return "file" }
func (d *fileDescriptor) Description() string { return "Sends a binary file response" }
func (d *fileDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return nil
}
func (d *fileDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"status":       map[string]any{"type": "number", "description": "HTTP status code (default 200)"},
			"data":         map[string]any{"description": "File data (bytes from storage.read)"},
			"content_type": map[string]any{"type": "string", "description": "MIME type for Content-Type header"},
			"filename":     map[string]any{"type": "string", "description": "Filename for Content-Disposition header (optional)"},
		},
		"required": []any{"data", "content_type"},
	}
}
func (d *fileDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "HTTP response set (binary file body)",
		"error":   "Expression evaluation error",
	}
}

type fileExecutor struct{}

func newFileExecutor(_ map[string]any) api.NodeExecutor { return &fileExecutor{} }

func (e *fileExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *fileExecutor) Execute(_ context.Context, nCtx api.ExecutionContext, config map[string]any, _ map[string]any) (string, any, error) {
	// Resolve status (default 200)
	status := 200
	if sv, ok := config["status"]; ok {
		if n, ok := plugin.ToInt(sv); ok {
			status = n
		} else if s, ok := sv.(string); ok && s != "" {
			resolved, err := nCtx.Resolve(s)
			if err != nil {
				return "", nil, fmt.Errorf("response.file: status: %w", err)
			}
			if n, ok := plugin.ToInt(resolved); ok {
				status = n
			}
		}
	}

	// Resolve data — expect []byte (from storage.read output)
	data, err := resolveBytes(nCtx, config)
	if err != nil {
		return "", nil, fmt.Errorf("response.file: %w", err)
	}

	// Resolve content_type
	contentType, err := plugin.ResolveString(nCtx, config, "content_type")
	if err != nil {
		return "", nil, fmt.Errorf("response.file: content_type: %w", err)
	}

	headers := map[string]string{
		"Content-Type": contentType,
	}

	// Resolve optional filename for Content-Disposition
	if fn, _ := config["filename"].(string); fn != "" {
		resolved, err := nCtx.Resolve(fn)
		if err != nil {
			return "", nil, fmt.Errorf("response.file: filename: %w", err)
		}
		if name, ok := resolved.(string); ok && name != "" {
			headers["Content-Disposition"] = fmt.Sprintf(`attachment; filename="%s"`, name)
		}
	}

	resp := &api.HTTPResponse{
		Status:  status,
		Headers: headers,
		Body:    data,
	}

	return api.OutputSuccess, resp, nil
}

func resolveBytes(nCtx api.ExecutionContext, config map[string]any) ([]byte, error) {
	raw, ok := config["data"]
	if !ok {
		return nil, fmt.Errorf("missing required field \"data\"")
	}

	switch v := raw.(type) {
	case []byte:
		return v, nil
	case string:
		resolved, err := nCtx.Resolve(v)
		if err != nil {
			return nil, fmt.Errorf("data: %w", err)
		}
		if b, ok := resolved.([]byte); ok {
			return b, nil
		}
		return nil, fmt.Errorf("data: expected bytes, got %T", resolved)
	default:
		return nil, fmt.Errorf("data: expected bytes, got %T", raw)
	}
}
