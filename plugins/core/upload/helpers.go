package upload

import (
	"context"
	"io"

	"github.com/chimpanze/noda/internal/plugin"
)

// storageWriter is the interface required by upload.handle.
// The storage service must implement WriteStream for streaming uploads.
type storageWriter interface {
	WriteStream(ctx context.Context, path string, r io.Reader) (int64, error)
}

func getStorageService(services map[string]any) (storageWriter, error) {
	return plugin.GetService[storageWriter](services, "destination")
}
