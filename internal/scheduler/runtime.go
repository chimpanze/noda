package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const (
	// maxHistoryEntries is the maximum number of job run records to retain.
	maxHistoryEntries = 1000
)

// ScheduleConfig holds the parsed configuration for a single schedule.
type ScheduleConfig struct {
	ID          string
	Cron        string
	Timezone    string
	Description string
	LockSvcName string // cache service name for distributed locking
	LockEnabled bool
	LockTTL     time.Duration
	Timeout     time.Duration // per-job execution timeout (default 5m)
	WorkflowID  string
	InputMap    map[string]any
}

// JobRun records a single execution attempt.
type JobRun struct {
	ScheduleID string
	TraceID    string
	StartedAt  time.Time
	Duration   time.Duration
	Success    bool
	Error      string
	Skipped    bool // true if lock was not acquired
}

// Runtime manages cron-based scheduled workflow execution.
type Runtime struct {
	schedules      []ScheduleConfig
	services       *registry.ServiceRegistry
	nodes          *registry.NodeRegistry
	workflows      map[string]map[string]any
	workflowCache  *engine.WorkflowCache
	compiler       *expr.Compiler
	tracer         oteltrace.Tracer
	logger         *slog.Logger
	secretsContext map[string]any

	cron    *cron.Cron
	mu      sync.RWMutex
	history []JobRun
}

// NewRuntime creates a new scheduler runtime.
// If compiler is nil, a new one is created.
func NewRuntime(
	schedules []ScheduleConfig,
	services *registry.ServiceRegistry,
	nodes *registry.NodeRegistry,
	workflows map[string]map[string]any,
	workflowCache *engine.WorkflowCache,
	compiler *expr.Compiler,
	tracer oteltrace.Tracer,
	logger *slog.Logger,
	secretsContext map[string]any,
) *Runtime {
	if logger == nil {
		logger = slog.Default()
	}
	if compiler == nil {
		compiler = expr.NewCompilerWithFunctions()
	}
	return &Runtime{
		schedules:      schedules,
		services:       services,
		nodes:          nodes,
		workflows:      workflows,
		workflowCache:  workflowCache,
		compiler:       compiler,
		tracer:         tracer,
		logger:         logger,
		secretsContext: secretsContext,
	}
}

// Start registers all cron jobs and begins the scheduler.
func (r *Runtime) Start() error {
	opts := []cron.Option{cron.WithSeconds()}
	r.cron = cron.New(opts...)

	for _, sc := range r.schedules {
		sc := sc
		spec := sc.Cron

		// Apply timezone if specified
		if sc.Timezone != "" {
			spec = "TZ=" + sc.Timezone + " " + spec
		}

		_, err := r.cron.AddFunc(spec, func() {
			r.runJob(sc)
		})
		if err != nil {
			return fmt.Errorf("scheduler: register job %q: %w", sc.ID, err)
		}

		r.logger.Info("scheduler: job registered",
			"schedule_id", sc.ID,
			"cron", sc.Cron,
			"timezone", sc.Timezone,
			"workflow", sc.WorkflowID,
		)
	}

	r.cron.Start()
	return nil
}

// Stop gracefully shuts down the scheduler and waits for running jobs to finish.
// If ctx is cancelled before all jobs finish, Stop returns ctx.Err().
func (r *Runtime) Stop(ctx context.Context) error {
	if r.cron == nil {
		return nil
	}
	cronCtx := r.cron.Stop()
	select {
	case <-cronCtx.Done():
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// History returns the job execution history (most recent first).
func (r *Runtime) History() []JobRun {
	r.mu.RLock()
	defer r.mu.RUnlock()
	n := len(r.history)
	result := make([]JobRun, n)
	for i, run := range r.history {
		result[n-1-i] = run
	}
	return result
}

// NextRun returns the next scheduled time for a job by ID.
// Entries are indexed in registration order matching r.schedules order.
func (r *Runtime) NextRun(scheduleID string) (time.Time, bool) {
	if r.cron == nil {
		return time.Time{}, false
	}
	entries := r.cron.Entries()
	for i, sc := range r.schedules {
		if sc.ID == scheduleID && i < len(entries) {
			return entries[i].Next, true
		}
	}
	return time.Time{}, false
}

// defaultJobTimeout is used when no per-schedule timeout is configured.
const defaultJobTimeout = 5 * time.Minute

// runJob executes a single scheduled job with optional distributed locking.
func (r *Runtime) runJob(sc ScheduleConfig) {
	start := time.Now()
	traceID := uuid.New().String()

	defer func() {
		if rv := recover(); rv != nil {
			r.logger.Error("scheduler: job panicked",
				"schedule_id", sc.ID,
				"panic", fmt.Sprintf("%v", rv),
			)
			r.recordRun(JobRun{
				ScheduleID: sc.ID,
				TraceID:    traceID,
				StartedAt:  start,
				Duration:   time.Since(start),
				Error:      fmt.Sprintf("panic: %v", rv),
			})
		}
	}()

	timeout := sc.Timeout
	if timeout == 0 {
		timeout = defaultJobTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	now := start

	r.logger.Info("scheduler: job firing",
		"schedule_id", sc.ID,
		"cron", sc.Cron,
		"trace_id", traceID,
	)

	// Distributed locking
	if sc.LockEnabled && sc.LockSvcName != "" {
		// Compute effective lock TTL without mutating the schedule config.
		// Ensure lock TTL outlives the job timeout (add 30s buffer for cleanup).
		lockTTL := sc.LockTTL
		if lockTTL > 0 && lockTTL < timeout+30*time.Second {
			lockTTL = timeout + 30*time.Second
		}

		lockKey := fmt.Sprintf("noda:schedule:%s:%d", sc.ID, now.Truncate(time.Minute).Unix())
		lockSvc, ok := r.services.Get(sc.LockSvcName)
		if !ok {
			r.logger.Error("scheduler: lock service not found",
				"schedule_id", sc.ID,
				"trace_id", traceID,
				"service", sc.LockSvcName,
			)
			r.recordRun(JobRun{
				ScheduleID: sc.ID,
				TraceID:    traceID,
				StartedAt:  start,
				Duration:   time.Since(start),
				Error:      fmt.Sprintf("lock service %q not found", sc.LockSvcName),
			})
			return
		}

		lockToken, err := tryAcquireLock(ctx, lockSvc, lockKey, lockTTL)
		if err != nil {
			r.logger.Error("scheduler: lock error",
				"schedule_id", sc.ID,
				"trace_id", traceID,
				"error", err.Error(),
			)
			r.recordRun(JobRun{
				ScheduleID: sc.ID,
				TraceID:    traceID,
				StartedAt:  start,
				Duration:   time.Since(start),
				Error:      err.Error(),
			})
			return
		}
		if lockToken == "" {
			r.logger.Info("scheduler: lock not acquired, skipping",
				"schedule_id", sc.ID,
				"trace_id", traceID,
			)
			r.recordRun(JobRun{
				ScheduleID: sc.ID,
				TraceID:    traceID,
				StartedAt:  start,
				Duration:   time.Since(start),
				Skipped:    true,
			})
			return
		}
		defer func() {
			// Use a fresh context for lock release since the job context may have expired.
			releaseCtx, releaseCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer releaseCancel()
			if err := releaseLockKey(releaseCtx, lockSvc, lockKey, lockToken); err != nil {
				r.logger.Warn("scheduler: lock release failed",
					"schedule_id", sc.ID,
					"trace_id", traceID,
					"error", err.Error(),
				)
			}
		}()
	}

	// Build trigger metadata for expressions
	scheduleMeta := map[string]any{
		"schedule": map[string]any{
			"id":   sc.ID,
			"cron": sc.Cron,
		},
	}

	// Resolve trigger input mapping
	input, err := engine.ResolveInput(r.compiler, sc.InputMap, scheduleMeta)
	if err != nil {
		r.logger.Error("scheduler: input mapping failed",
			"schedule_id", sc.ID,
			"trace_id", traceID,
			"error", err.Error(),
		)
		r.recordRun(JobRun{
			ScheduleID: sc.ID,
			TraceID:    traceID,
			StartedAt:  start,
			Duration:   time.Since(start),
			Error:      err.Error(),
		})
		return
	}

	opts := []engine.ExecutionContextOption{
		engine.WithInput(input),
		engine.WithTrigger(api.TriggerData{
			Type:      "schedule",
			Timestamp: now,
			TraceID:   traceID,
		}),
		engine.WithWorkflowID(sc.WorkflowID),
		engine.WithLogger(r.logger),
		engine.WithCompiler(r.compiler),
		engine.WithSecrets(r.secretsContext),
	}
	if r.tracer != nil {
		opts = append(opts, engine.WithTracer(r.tracer))
	}
	execCtx := engine.NewExecutionContext(opts...)

	wfErr := engine.RunWorkflow(ctx, sc.WorkflowID, execCtx, r.workflowCache, r.workflows, r.services, r.nodes)

	run := JobRun{
		ScheduleID: sc.ID,
		TraceID:    traceID,
		StartedAt:  start,
		Duration:   time.Since(start),
		Success:    wfErr == nil,
	}
	if wfErr != nil {
		run.Error = wfErr.Error()
		r.logger.Error("scheduler: job failed",
			"schedule_id", sc.ID,
			"trace_id", traceID,
			"duration", run.Duration.String(),
			"error", wfErr.Error(),
		)
	} else {
		r.logger.Info("scheduler: job completed",
			"schedule_id", sc.ID,
			"trace_id", traceID,
			"duration", run.Duration.String(),
		)
	}
	r.recordRun(run)
}

// recordRun appends a job run to history (capped at maxHistoryEntries).
func (r *Runtime) recordRun(run JobRun) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.history = append(r.history, run)
	if len(r.history) > maxHistoryEntries {
		// Drop the oldest entry (front of the slice)
		r.history = r.history[len(r.history)-maxHistoryEntries:]
	}
}

// ParseScheduleConfigs extracts ScheduleConfig from raw config maps.
func ParseScheduleConfigs(schedules map[string]map[string]any) []ScheduleConfig {
	var configs []ScheduleConfig
	for _, raw := range schedules {
		tz := engine.MapStrVal(raw, "timezone")
		if tz != "" {
			if _, err := time.LoadLocation(tz); err != nil {
				slog.Warn("scheduler: invalid timezone, falling back to server default",
					"schedule_id", engine.MapStrVal(raw, "id"),
					"timezone", tz,
					"error", err,
				)
				tz = ""
			}
		}

		sc := ScheduleConfig{
			ID:          engine.MapStrVal(raw, "id"),
			Cron:        engine.MapStrVal(raw, "cron"),
			Timezone:    tz,
			Description: engine.MapStrVal(raw, "description"),
		}

		if svc, ok := raw["services"].(map[string]any); ok {
			sc.LockSvcName = engine.MapStrVal(svc, "lock")
		}

		if trigger, ok := raw["trigger"].(map[string]any); ok {
			sc.WorkflowID = engine.MapStrVal(trigger, "workflow")
			if input, ok := trigger["input"].(map[string]any); ok {
				sc.InputMap = input
			}
		}

		if lock, ok := raw["lock"].(map[string]any); ok {
			if enabled, ok := lock["enabled"].(bool); ok {
				sc.LockEnabled = enabled
			}
			if ttlStr, ok := lock["ttl"].(string); ok {
				if d, err := time.ParseDuration(ttlStr); err == nil {
					sc.LockTTL = d
				}
			}
		}

		if timeoutStr, ok := raw["timeout"].(string); ok {
			if d, err := time.ParseDuration(timeoutStr); err == nil {
				sc.Timeout = d
			}
		}

		configs = append(configs, sc)
	}
	return configs
}
