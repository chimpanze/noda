package server

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

// TriggerResult holds the mapped trigger data ready for workflow execution.
type TriggerResult struct {
	Input   map[string]any
	Auth    *api.AuthData
	Trigger api.TriggerData
}

// MapTrigger evaluates trigger.input expressions against raw request data.
func MapTrigger(c fiber.Ctx, triggerConfig map[string]any, compiler *expr.Compiler) (*TriggerResult, error) {
	// Build raw request context for expression evaluation
	rawCtx := buildRawRequestContext(c)

	// Build trigger metadata
	traceID := uuid.New().String()
	if rid := c.Get("X-Request-Id"); rid != "" {
		traceID = rid
	}

	result := &TriggerResult{
		Input: make(map[string]any),
		Trigger: api.TriggerData{
			Type:      "http",
			Timestamp: time.Now(),
			TraceID:   traceID,
		},
	}

	// Handle raw_body preservation
	if rawBody, ok := triggerConfig["raw_body"].(bool); ok && rawBody {
		result.Trigger = api.TriggerData{
			Type:      result.Trigger.Type,
			Timestamp: result.Trigger.Timestamp,
			TraceID:   result.Trigger.TraceID,
		}
		// Store raw body bytes — read from request before parsing
		rawBodyBytes := c.Body()
		rawCtx["raw_body"] = string(rawBodyBytes)
	}

	// Evaluate input expressions
	inputMap, ok := triggerConfig["input"].(map[string]any)
	if ok {
		// Get file fields (to skip expression resolution)
		fileFields := getFileFields(triggerConfig)

		resolver := expr.NewResolver(compiler, rawCtx)
		for key, exprVal := range inputMap {
			// Skip file fields — pass raw streams
			if fileFields[key] {
				formFile, err := c.FormFile(key)
				if err != nil {
					return nil, fmt.Errorf("trigger mapping: file field %q: %w", key, err)
				}
				result.Input[key] = formFile
				continue
			}

			exprStr, ok := exprVal.(string)
			if !ok {
				result.Input[key] = exprVal
				continue
			}

			resolved, err := resolver.Resolve(exprStr)
			if err != nil {
				return nil, fmt.Errorf("trigger mapping: field %q: %w", key, err)
			}
			result.Input[key] = resolved
		}
	}

	// Populate auth from JWT middleware
	result.Auth = extractAuth(c)

	return result, nil
}

// buildRawRequestContext creates the expression evaluation context from the raw HTTP request.
func buildRawRequestContext(c fiber.Ctx) map[string]any {
	ctx := map[string]any{
		"request": map[string]any{
			"body":    parseBody(c),
			"params":  parseParams(c),
			"query":   parseQuery(c),
			"headers": parseHeaders(c),
			"method":  c.Method(),
			"path":    c.Path(),
		},
	}
	return ctx
}

func parseBody(c fiber.Ctx) any {
	contentType := c.Get("Content-Type")
	body := c.Body()
	if len(body) == 0 {
		return nil
	}

	// Try JSON first
	if contentType == "" || containsStr(contentType, "json") {
		var parsed any
		if err := json.Unmarshal(body, &parsed); err == nil {
			return parsed
		}
	}

	// Try form data
	if containsStr(contentType, "form") {
		form := make(map[string]any)
		c.Request().PostArgs().VisitAll(func(key, value []byte) {
			form[string(key)] = string(value)
		})
		if len(form) > 0 {
			return form
		}
		// Try multipart
		mf, err := c.MultipartForm()
		if err == nil && mf != nil {
			for k, v := range mf.Value {
				if len(v) == 1 {
					form[k] = v[0]
				} else {
					form[k] = v
				}
			}
			return form
		}
	}

	// Return raw string as fallback
	return string(body)
}

func parseParams(c fiber.Ctx) map[string]any {
	params := make(map[string]any)
	// Fiber v3: use Route().Params to get param names, then Params() to get values
	for _, param := range c.Route().Params {
		params[param] = c.Params(param)
	}
	return params
}

func parseQuery(c fiber.Ctx) map[string]any {
	query := make(map[string]any)
	for k, v := range c.Queries() {
		query[k] = v
	}
	return query
}

func parseHeaders(c fiber.Ctx) map[string]any {
	headers := make(map[string]any)
	for k, v := range c.GetReqHeaders() {
		if len(v) == 1 {
			headers[k] = v[0]
		} else if len(v) > 1 {
			headers[k] = v[0]
		}
	}
	return headers
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// extractAuth reads JWT claims from Fiber locals (set by JWT middleware).
func extractAuth(c fiber.Ctx) *api.AuthData {
	claims, _ := c.Locals("jwt_claims").(map[string]any)
	if claims == nil {
		return nil
	}

	auth := &api.AuthData{
		Claims: claims,
	}
	if userID, ok := c.Locals("jwt_user_id").(string); ok {
		auth.UserID = userID
	}
	if roles, ok := c.Locals("jwt_roles").([]string); ok {
		auth.Roles = roles
	}

	return auth
}

// getFileFields returns a set of field names that should be treated as file streams.
func getFileFields(triggerConfig map[string]any) map[string]bool {
	fields := make(map[string]bool)
	if files, ok := triggerConfig["files"].([]any); ok {
		for _, f := range files {
			if name, ok := f.(string); ok {
				fields[name] = true
			}
		}
	}
	return fields
}

// ReadRawBody reads and returns the raw body bytes for webhook signature verification.
func ReadRawBody(c fiber.Ctx) ([]byte, error) {
	return io.ReadAll(c.Request().BodyStream())
}
