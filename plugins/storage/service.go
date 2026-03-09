package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/spf13/afero"
)

// Service wraps an Afero filesystem and implements api.StorageService.
type Service struct {
	fs      afero.Fs
	backend string
}

// Fs returns the underlying Afero filesystem (for direct access by upload node).
func (s *Service) Fs() afero.Fs { return s.fs }

// Read reads file contents from the given path.
func (s *Service) Read(_ context.Context, path string) ([]byte, error) {
	f, err := s.fs.Open(path)
	if err != nil {
		if isNotExist(err) {
			return nil, &api.NotFoundError{Resource: "storage", ID: path}
		}
		return nil, fmt.Errorf("storage read %q: %w", path, err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("storage read %q: %w", path, err)
	}
	return data, nil
}

// Write writes data to the given path, creating parent directories as needed.
func (s *Service) Write(_ context.Context, path string, data []byte) error {
	dir := filepath.Dir(path)
	if dir != "." && dir != "/" {
		if err := s.fs.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("storage write %q: mkdir: %w", path, err)
		}
	}

	f, err := s.fs.Create(path)
	if err != nil {
		return fmt.Errorf("storage write %q: create: %w", path, err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("storage write %q: write: %w", path, err)
	}
	return nil
}

// WriteStream streams data from an io.Reader to the given path.
func (s *Service) WriteStream(_ context.Context, path string, r io.Reader) (int64, error) {
	dir := filepath.Dir(path)
	if dir != "." && dir != "/" {
		if err := s.fs.MkdirAll(dir, 0755); err != nil {
			return 0, fmt.Errorf("storage write stream %q: mkdir: %w", path, err)
		}
	}

	f, err := s.fs.Create(path)
	if err != nil {
		return 0, fmt.Errorf("storage write stream %q: create: %w", path, err)
	}
	defer f.Close()

	n, err := io.Copy(f, r)
	if err != nil {
		return n, fmt.Errorf("storage write stream %q: copy: %w", path, err)
	}
	return n, nil
}

// Delete removes a file at the given path.
func (s *Service) Delete(_ context.Context, path string) error {
	err := s.fs.Remove(path)
	if err != nil {
		if isNotExist(err) {
			return &api.NotFoundError{Resource: "storage", ID: path}
		}
		return fmt.Errorf("storage delete %q: %w", path, err)
	}
	return nil
}

// List returns all file paths under the given prefix.
func (s *Service) List(_ context.Context, prefix string) ([]string, error) {
	var paths []string

	err := afero.Walk(s.fs, prefix, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if isNotExist(err) {
				return nil // prefix doesn't exist — return empty list
			}
			return err
		}
		if !info.IsDir() {
			// Normalize to forward slashes
			paths = append(paths, strings.ReplaceAll(path, "\\", "/"))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("storage list %q: %w", prefix, err)
	}

	return paths, nil
}

// isNotExist checks if an error is a "not found" error.
func isNotExist(err error) bool {
	return strings.Contains(err.Error(), "no such file") ||
		strings.Contains(err.Error(), "not found") ||
		strings.Contains(err.Error(), "does not exist")
}

// Verify Service implements StorageService at compile time.
var _ api.StorageService = (*Service)(nil)
