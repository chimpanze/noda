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
	schedules []ScheduleConfig
	services  *registry.ServiceRegistry
	nodes     *registry.NodeRegistry
	workflows map[string]map[string]any
	compiler  *expr.Compiler
	logger    *slog.Logger

	cron    *cron.Cron
	mu      sync.RWMutex
	history []JobRun
}

// NewRuntime creates a new scheduler runtime.
func NewRuntime(
	schedules []ScheduleConfig,
	services *registry.ServiceRegistry,
	nodes *registry.NodeRegistry,
	workflows map[string]map[string]any,
	logger *slog.Logger,
) *Runtime {
	if logger == nil {
		logger = slog.Default()
	}
	return &Runtime{
		schedules: schedules,
		services:  services,
		nodes:     nodes,
		workflows: workflows,
		compiler:  expr.NewCompilerWithFunctions(),
		logger:    logger,
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
func (r *Runtime) Stop() {
	if r.cron != nil {
		ctx := r.cron.Stop()
		<-ctx.Done()
	}
}

// History returns the job execution history (most recent first).
func (r *Runtime) History() []JobRun {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]JobRun, len(r.history))
	copy(result, r.history)
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

// runJob executes a single scheduled job with optional distributed locking.
func (r *Runtime) runJob(sc ScheduleConfig) {
	now := time.Now()
	traceID := uuid.New().String()
	start := now

	r.logger.Info("scheduler: job firing",
		"schedule_id", sc.ID,
		"cron", sc.Cron,
		"trace_id", traceID,
	)

	// Distributed locking
	if sc.LockEnabled && sc.LockSvcName != "" {
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

		acquired, err := tryAcquireLock(lockSvc, lockKey, sc.LockTTL)
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
		if !acquired {
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
			if err := releaseLockKey(lockSvc, lockKey); err != nil {
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
	input, err := r.resolveInput(sc.InputMap, scheduleMeta)
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

	execCtx := engine.NewExecutionContext(
		engine.WithInput(input),
		engine.WithTrigger(api.TriggerData{
			Type:      "schedule",
			Timestamp: now,
			TraceID:   traceID,
		}),
		engine.WithWorkflowID(sc.WorkflowID),
		engine.WithLogger(r.logger),
		engine.WithCompiler(r.compiler),
	)

	wfErr := r.executeWorkflow(context.Background(), sc.WorkflowID, execCtx)

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

// executeWorkflow compiles and runs a workflow.
func (r *Runtime) executeWorkflow(ctx context.Context, workflowID string, execCtx *engine.ExecutionContextImpl) error {
	wfData, ok := r.workflows[workflowID]
	if !ok {
		return fmt.Errorf("workflow %q not found", workflowID)
	}

	wfConfig := parseWorkflowConfig(workflowID, wfData)
	graph, err := engine.Compile(wfConfig, r.nodes)
	if err != nil {
		return fmt.Errorf("compile workflow %q: %w", workflowID, err)
	}
	return engine.ExecuteGraph(ctx, graph, execCtx, r.services, r.nodes)
}

// resolveInput evaluates trigger input mapping against schedule metadata.
func (r *Runtime) resolveInput(inputMap map[string]any, scheduleCtx map[string]any) (map[string]any, error) {
	if inputMap == nil {
		return map[string]any{}, nil
	}
	resolver := expr.NewResolver(r.compiler, scheduleCtx)
	result := make(map[string]any)
	for key, exprVal := range inputMap {
		exprStr, ok := exprVal.(string)
		if !ok {
			result[key] = exprVal
			continue
		}
		resolved, err := resolver.Resolve(exprStr)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", key, err)
		}
		result[key] = resolved
	}
	return result, nil
}

// recordRun appends a job run to history (capped at 1000 entries).
func (r *Runtime) recordRun(run JobRun) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.history = append([]JobRun{run}, r.history...)
	if len(r.history) > 1000 {
		r.history = r.history[:1000]
	}
}

// parseWorkflowConfig converts raw workflow config to engine.WorkflowConfig.
func parseWorkflowConfig(id string, raw map[string]any) engine.WorkflowConfig {
	wf := engine.WorkflowConfig{
		ID:    id,
		Nodes: make(map[string]engine.NodeConfig),
	}

	nodesRaw, _ := raw["nodes"].(map[string]any)
	for nodeID, nodeRaw := range nodesRaw {
		nm, ok := nodeRaw.(map[string]any)
		if !ok {
			continue
		}
		nc := engine.NodeConfig{
			Type: mapStrVal(nm, "type"),
			As:   mapStrVal(nm, "as"),
		}
		if cfg, ok := nm["config"].(map[string]any); ok {
			nc.Config = cfg
		}
		if svc, ok := nm["services"].(map[string]any); ok {
			nc.Services = make(map[string]string)
			for k, v := range svc {
				nc.Services[k] = fmt.Sprintf("%v", v)
			}
		}
		wf.Nodes[nodeID] = nc
	}

	edgesRaw, _ := raw["edges"].([]any)
	for _, edgeRaw := range edgesRaw {
		em, ok := edgeRaw.(map[string]any)
		if !ok {
			continue
		}
		wf.Edges = append(wf.Edges, engine.EdgeConfig{
			From:   mapStrVal(em, "from"),
			To:     mapStrVal(em, "to"),
			Output: mapStrVal(em, "output"),
		})
	}
	return wf
}

func mapStrVal(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

// ParseScheduleConfigs extracts ScheduleConfig from raw config maps.
func ParseScheduleConfigs(schedules map[string]map[string]any) []ScheduleConfig {
	var configs []ScheduleConfig
	for _, raw := range schedules {
		sc := ScheduleConfig{
			ID:          mapStrVal(raw, "id"),
			Cron:        mapStrVal(raw, "cron"),
			Timezone:    mapStrVal(raw, "timezone"),
			Description: mapStrVal(raw, "description"),
		}

		if svc, ok := raw["services"].(map[string]any); ok {
			sc.LockSvcName = mapStrVal(svc, "lock")
		}

		if trigger, ok := raw["trigger"].(map[string]any); ok {
			sc.WorkflowID = mapStrVal(trigger, "workflow")
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

		configs = append(configs, sc)
	}
	return configs
}
