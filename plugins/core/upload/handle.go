package upload

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
)

type handleDescriptor struct{}

func (d *handleDescriptor) Name() string { return "handle" }
func (d *handleDescriptor) Description() string {
	return "Handles multipart file uploads with validation"
}

func (d *handleDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{
		"destination": {Prefix: "storage", Required: true},
	}
}

func (d *handleDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"max_size":      map[string]any{"type": "number", "description": "Maximum file size in bytes"},
			"allowed_types": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Allowed MIME type patterns (supports wildcards)"},
			"max_files":     map[string]any{"type": "number", "description": "Maximum number of files to accept"},
			"path":          map[string]any{"type": "string", "description": "Storage destination path"},
			"field":         map[string]any{"type": "string", "description": "Form field name (default: file)"},
		},
		"required": []any{"max_size", "allowed_types", "path"},
	}
}
func (d *handleDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "Object with filename, size, content_type, and file data",
		"error":   "Upload validation error (size/type)",
	}
}

type handleExecutor struct{}

func newHandleExecutor(_ map[string]any) api.NodeExecutor { return &handleExecutor{} }

func (e *handleExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *handleExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	// Get destination storage service
	destSvc, err := getStorageService(services)
	if err != nil {
		return "", nil, err
	}

	// Parse static config values
	maxSize := int64(10 * 1024 * 1024) // default 10MB
	if v, ok := plugin.ToInt64(config["max_size"]); ok {
		maxSize = v
	}

	maxFiles := 1
	if v, ok := plugin.ToInt(config["max_files"]); ok {
		maxFiles = v
	}

	allowedTypes := parseStringSlice(config["allowed_types"])

	// Resolve path expression
	pathExpr, _ := config["path"].(string)
	resolvedPath, err := nCtx.Resolve(pathExpr)
	if err != nil {
		return "", nil, fmt.Errorf("upload.handle: resolve path: %w", err)
	}
	storagePath, ok := resolvedPath.(string)
	if !ok {
		return "", nil, fmt.Errorf("upload.handle: path must resolve to string, got %T", resolvedPath)
	}

	// Get field name for lookup in input (default: "file")
	fieldName := "file"
	if f, ok := config["field"].(string); ok && f != "" {
		fieldName = f
	}

	// Extract file(s) from input
	files, err := extractFiles(nCtx, fieldName, maxFiles)
	if err != nil {
		return "", nil, &api.ValidationError{Field: fieldName, Message: err.Error()}
	}

	if len(files) == 0 {
		return "", nil, &api.ValidationError{Field: fieldName, Message: "no files uploaded"}
	}

	// Process files and return metadata for the first (or only) file
	var results []map[string]any
	for i, fh := range files {
		// Quick reject if FileHeader.Size is populated and exceeds limit
		if fh.Size > 0 && fh.Size > maxSize {
			return "", nil, &api.ValidationError{
				Field:   fieldName,
				Message: fmt.Sprintf("file %d exceeds max size of %d bytes (got %d)", i, maxSize, fh.Size),
			}
		}

		// Open and read content (up to maxSize+1 bytes for size enforcement)
		f, err := fh.Open()
		if err != nil {
			return "", nil, fmt.Errorf("upload.handle: open file: %w", err)
		}

		// Read up to maxSize+1 bytes to detect oversized files
		lr := &limitedReader{R: f, N: maxSize + 1}
		content, readErr := io.ReadAll(lr)
		_ = f.Close()
		if readErr != nil {
			return "", nil, fmt.Errorf("upload.handle: read file: %w", readErr)
		}
		if lr.exceeded {
			return "", nil, &api.ValidationError{
				Field:   fieldName,
				Message: fmt.Sprintf("file %d exceeds max size of %d bytes", i, maxSize),
			}
		}

		// Detect content type from the first 512 bytes
		sniffLen := 512
		if len(content) < sniffLen {
			sniffLen = len(content)
		}
		detectedType := http.DetectContentType(content[:sniffLen])

		// Validate MIME type
		if len(allowedTypes) > 0 && !mimeAllowed(detectedType, allowedTypes) {
			return "", nil, &api.ValidationError{
				Field:   fieldName,
				Message: fmt.Sprintf("file type %q not allowed", detectedType),
			}
		}

		// Determine storage path for this file (append index if multiple)
		filePath := storagePath
		if len(files) > 1 {
			filePath = fmt.Sprintf("%s_%d", storagePath, i)
		}

		// Write to storage
		written, err := destSvc.WriteStream(ctx, filePath, bytes.NewReader(content))
		if err != nil {
			return "", nil, fmt.Errorf("upload.handle: write to storage: %w", err)
		}

		results = append(results, map[string]any{
			"path":         filePath,
			"size":         written,
			"content_type": detectedType,
			"filename":     fh.Filename,
		})
	}

	// Return single object for single file, array for multiple
	if len(results) == 1 {
		return api.OutputSuccess, results[0], nil
	}
	return api.OutputSuccess, map[string]any{"files": results}, nil
}

// extractFiles pulls file headers from the execution context input.
func extractFiles(nCtx api.ExecutionContext, field string, maxFiles int) ([]*multipart.FileHeader, error) {
	input, ok := nCtx.Input().(map[string]any)
	if !ok {
		return nil, fmt.Errorf("input is not a map")
	}

	raw, ok := input[field]
	if !ok {
		return nil, fmt.Errorf("field %q not found in input", field)
	}

	switch v := raw.(type) {
	case *multipart.FileHeader:
		return []*multipart.FileHeader{v}, nil
	case []*multipart.FileHeader:
		if len(v) > maxFiles {
			return nil, fmt.Errorf("too many files: got %d, max %d", len(v), maxFiles)
		}
		return v, nil
	default:
		return nil, fmt.Errorf("field %q is not a file upload (got %T)", field, raw)
	}
}

func mimeAllowed(detected string, allowed []string) bool {
	for _, a := range allowed {
		if a == detected || a == "*" {
			return true
		}
		// Check prefix match (e.g., "image/*")
		if len(a) > 1 && a[len(a)-1] == '*' {
			prefix := a[:len(a)-1]
			if len(detected) >= len(prefix) && detected[:len(prefix)] == prefix {
				return true
			}
		}
	}
	return false
}

func parseStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

// limitedReader wraps an io.Reader and sets exceeded=true if more than N bytes are read.
type limitedReader struct {
	R        io.Reader
	N        int64
	exceeded bool
}

func (l *limitedReader) Read(p []byte) (int, error) {
	if l.N <= 0 {
		l.exceeded = true
		return 0, io.EOF
	}
	if int64(len(p)) > l.N {
		p = p[:l.N]
	}
	n, err := l.R.Read(p)
	l.N -= int64(n)
	if l.N <= 0 && err == nil {
		l.exceeded = true
		return n, io.EOF
	}
	return n, err
}
