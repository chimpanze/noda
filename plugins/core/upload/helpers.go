package upload

import (
	"context"
	"fmt"
	"io"
)

// storageWriter is the interface required by upload.handle.
// The storage service must implement WriteStream for streaming uploads.
type storageWriter interface {
	WriteStream(ctx context.Context, path string, r io.Reader) (int64, error)
}

func getStorageService(services map[string]any) (storageWriter, error) {
	svc, ok := services["destination"]
	if !ok {
		return nil, fmt.Errorf("upload.handle: destination storage service not configured")
	}
	sw, ok := svc.(storageWriter)
	if !ok {
		return nil, fmt.Errorf("upload.handle: destination service does not implement WriteStream")
	}
	return sw, nil
}
