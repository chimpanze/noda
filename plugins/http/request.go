package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
)

type requestDescriptor struct{}

func (d *requestDescriptor) Name() string                           { return "request" }
func (d *requestDescriptor) Description() string                    { return "Makes an outbound HTTP request" }
func (d *requestDescriptor) ServiceDeps() map[string]api.ServiceDep { return httpServiceDeps }
func (d *requestDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"method":  map[string]any{"type": "string", "description": "HTTP method (GET, POST, PUT, etc.)"},
			"url":     map[string]any{"type": "string", "description": "Request URL"},
			"headers": map[string]any{"type": "object", "description": "Request headers"},
			"body":    map[string]any{"description": "Request body"},
			"timeout": map[string]any{"type": "string", "description": "Per-request timeout override"},
		},
		"required": []any{"method", "url"},
	}
}

type requestExecutor struct{}

func newRequestExecutor(_ map[string]any) api.NodeExecutor { return &requestExecutor{} }

func (e *requestExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *requestExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[*Service](services, "client")
	if err != nil {
		return "", nil, err
	}
	return doRequest(ctx, nCtx, config, svc, "")
}

// doRequest is the shared implementation for http.request, http.get, http.post.
func doRequest(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, svc *Service, fixedMethod string) (string, any, error) {
	// Resolve method
	method := fixedMethod
	if method == "" {
		m, err := plugin.ResolveString(nCtx, config, "method")
		if err != nil {
			return "", nil, fmt.Errorf("http.request: %w", err)
		}
		method = strings.ToUpper(m)
	}

	// Resolve URL
	url, err := plugin.ResolveString(nCtx, config, "url")
	if err != nil {
		return "", nil, fmt.Errorf("http.request: %w", err)
	}
	if svc.baseURL != "" && !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = strings.TrimRight(svc.baseURL, "/") + "/" + strings.TrimLeft(url, "/")
	}

	// Resolve headers
	headers, err := plugin.ResolveHeaders(nCtx, config)
	if err != nil {
		return "", nil, fmt.Errorf("http.request: %w", err)
	}

	// Resolve body
	var bodyReader io.Reader
	if bodyVal, ok, _ := plugin.ResolveOptionalAny(nCtx, config, "body"); ok && bodyVal != nil {
		switch v := bodyVal.(type) {
		case string:
			bodyReader = strings.NewReader(v)
		case []byte:
			bodyReader = bytes.NewReader(v)
		default:
			jsonBytes, err := json.Marshal(v)
			if err != nil {
				return "", nil, fmt.Errorf("http.request: marshal body: %w", err)
			}
			bodyReader = bytes.NewReader(jsonBytes)
			if headers == nil {
				headers = make(map[string]string)
			}
			if _, ok := headers["Content-Type"]; !ok {
				headers["Content-Type"] = "application/json"
			}
		}
	}

	// Per-request timeout
	if timeoutStr, ok, _ := plugin.ResolveOptionalString(nCtx, config, "timeout"); ok {
		d, err := time.ParseDuration(timeoutStr)
		if err != nil {
			return "", nil, fmt.Errorf("http.request: invalid timeout %q: %w", timeoutStr, err)
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, d)
		defer cancel()
	}

	// Build request
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return "", nil, fmt.Errorf("http.request: create request: %w", err)
	}

	// Apply default headers first, then custom headers
	for k, v := range svc.defaultHeaders {
		req.Header.Set(k, v)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Execute request
	resp, err := svc.client.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", nil, &api.TimeoutError{
				Operation: fmt.Sprintf("HTTP %s %s", method, url),
			}
		}
		return "", nil, fmt.Errorf("http.request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, fmt.Errorf("http.request: read response: %w", err)
	}

	// Build response headers map
	respHeaders := make(map[string]any, len(resp.Header))
	for k, v := range resp.Header {
		if len(v) == 1 {
			respHeaders[strings.ToLower(k)] = v[0]
		} else {
			respHeaders[strings.ToLower(k)] = v
		}
	}

	// Auto-detect JSON response
	var body any
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") || json.Valid(respBody) {
		var parsed any
		if err := json.Unmarshal(respBody, &parsed); err == nil {
			body = parsed
		} else {
			body = string(respBody)
		}
	} else {
		body = string(respBody)
	}

	result := map[string]any{
		"status":  resp.StatusCode,
		"headers": respHeaders,
		"body":    body,
	}

	return api.OutputSuccess, result, nil
}
