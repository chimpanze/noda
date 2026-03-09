package devmode

import (
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher monitors config directories for changes with debouncing.
type Watcher struct {
	watcher  *fsnotify.Watcher
	logger   *slog.Logger
	onChange func(path string)
	debounce time.Duration
	done     chan struct{}
	wg       sync.WaitGroup
}

// NewWatcher creates a file watcher that calls onChange after debounce silence.
func NewWatcher(onChange func(path string), logger *slog.Logger) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &Watcher{
		watcher:  fsw,
		logger:   logger,
		onChange: onChange,
		debounce: 100 * time.Millisecond,
		done:     make(chan struct{}),
	}, nil
}

// WatchDir recursively watches a directory and all subdirectories.
func (w *Watcher) WatchDir(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip inaccessible
		}
		if info.IsDir() {
			// Skip hidden directories
			if len(info.Name()) > 1 && info.Name()[0] == '.' {
				return filepath.SkipDir
			}
			if err := w.watcher.Add(path); err != nil {
				w.logger.Warn("watcher: failed to watch directory", "path", path, "error", err.Error())
			}
		}
		return nil
	})
}

// Start begins watching for events. Call Stop to shut down.
func (w *Watcher) Start() {
	w.wg.Add(1)
	go w.loop()
}

// Stop shuts down the watcher and waits for cleanup.
func (w *Watcher) Stop() {
	close(w.done)
	w.watcher.Close()
	w.wg.Wait()
}

func (w *Watcher) loop() {
	defer w.wg.Done()

	var timer *time.Timer
	var lastPath string

	for {
		select {
		case <-w.done:
			if timer != nil {
				timer.Stop()
			}
			return

		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			// Only react to write/create/rename operations on JSON files
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
				continue
			}
			ext := filepath.Ext(event.Name)
			if ext != ".json" {
				continue
			}

			lastPath = event.Name
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(w.debounce, func() {
				w.logger.Info("config file changed", "path", lastPath)
				w.onChange(lastPath)
			})

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.logger.Error("watcher error", "error", err.Error())
		}
	}
}
