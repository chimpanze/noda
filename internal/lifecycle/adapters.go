package lifecycle

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/connmgr"
	"github.com/chimpanze/noda/internal/devmode"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/internal/scheduler"
	"github.com/chimpanze/noda/internal/server"
	"github.com/chimpanze/noda/internal/trace"
	"github.com/chimpanze/noda/internal/wasm"
	"github.com/chimpanze/noda/internal/worker"
)

// serverComponent wraps *server.Server.
// Start is a no-op because srv.Start() blocks and is called separately.
// Stop calls srv.Stop() for graceful shutdown.
type serverComponent struct {
	srv *server.Server
}

func ServerComponent(srv *server.Server) Component       { return &serverComponent{srv: srv} }
func (c *serverComponent) Name() string                  { return "http-server" }
func (c *serverComponent) Start(_ context.Context) error { return nil }
func (c *serverComponent) Stop(ctx context.Context) error { return c.srv.Stop(ctx) }

// workerComponent wraps *worker.Runtime.
type workerComponent struct {
	rt *worker.Runtime
}

func WorkerComponent(rt *worker.Runtime) Component        { return &workerComponent{rt: rt} }
func (c *workerComponent) Name() string                   { return "workers" }
func (c *workerComponent) Start(ctx context.Context) error { return c.rt.Start(ctx) }
func (c *workerComponent) Stop(ctx context.Context) error  { return c.rt.Stop(ctx) }

// schedulerComponent wraps *scheduler.Runtime.
type schedulerComponent struct {
	rt *scheduler.Runtime
}

func SchedulerComponent(rt *scheduler.Runtime) Component    { return &schedulerComponent{rt: rt} }
func (c *schedulerComponent) Name() string                  { return "scheduler" }
func (c *schedulerComponent) Start(_ context.Context) error { return c.rt.Start() }
func (c *schedulerComponent) Stop(ctx context.Context) error { return c.rt.Stop(ctx) }

// wasmComponent wraps *wasm.Runtime.
type wasmComponent struct {
	rt *wasm.Runtime
}

func WasmComponent(rt *wasm.Runtime) Component          { return &wasmComponent{rt: rt} }
func (c *wasmComponent) Name() string                   { return "wasm" }
func (c *wasmComponent) Start(ctx context.Context) error { return c.rt.StartAll(ctx) }
func (c *wasmComponent) Stop(ctx context.Context) error {
	c.rt.StopAll(ctx)
	return nil
}

// connManagerComponent wraps *connmgr.ManagerGroup.
type connManagerComponent struct {
	mg *connmgr.ManagerGroup
}

func ConnManagerComponent(mg *connmgr.ManagerGroup) Component { return &connManagerComponent{mg: mg} }
func (c *connManagerComponent) Name() string                  { return "connections" }
func (c *connManagerComponent) Start(_ context.Context) error { return nil }
func (c *connManagerComponent) Stop(ctx context.Context) error { return c.mg.Stop(ctx) }

// serviceRegistryComponent wraps *registry.ServiceRegistry.
type serviceRegistryComponent struct {
	sr *registry.ServiceRegistry
}

func ServiceRegistryComponent(sr *registry.ServiceRegistry) Component {
	return &serviceRegistryComponent{sr: sr}
}
func (c *serviceRegistryComponent) Name() string                  { return "services" }
func (c *serviceRegistryComponent) Start(_ context.Context) error { return nil }
func (c *serviceRegistryComponent) Stop(_ context.Context) error {
	if errs := c.sr.ShutdownAll(); len(errs) > 0 {
		return fmt.Errorf("%d service shutdown errors: %v", len(errs), errs)
	}
	return nil
}

// tracerComponent wraps *trace.Provider.
type tracerComponent struct {
	tp *trace.Provider
}

func TracerComponent(tp *trace.Provider) Component       { return &tracerComponent{tp: tp} }
func (c *tracerComponent) Name() string                  { return "tracer" }
func (c *tracerComponent) Start(_ context.Context) error { return nil }
func (c *tracerComponent) Stop(ctx context.Context) error { return c.tp.Shutdown(ctx) }

// watcherComponent wraps *devmode.Watcher.
type watcherComponent struct {
	w *devmode.Watcher
}

func WatcherComponent(w *devmode.Watcher) Component      { return &watcherComponent{w: w} }
func (c *watcherComponent) Name() string                  { return "file-watcher" }
func (c *watcherComponent) Start(_ context.Context) error {
	c.w.Start()
	return nil
}
func (c *watcherComponent) Stop(_ context.Context) error {
	c.w.Stop()
	return nil
}
