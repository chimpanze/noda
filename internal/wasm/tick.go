package wasm

import (
	"fmt"
	"time"
)

// tickLoop runs the tick loop at the configured rate.
func (m *Module) tickLoop() {
	interval := time.Duration(1000/m.tickRate) * time.Millisecond
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return

		case <-ticker.C:
			m.executeTick()

		case req := <-m.queryCh:
			// Process query/command between ticks
			m.processQuery(req)
		}
	}
}

// executeTick runs a single tick.
func (m *Module) executeTick() {
	now := time.Now()

	m.mu.Lock()
	dt := now.Sub(m.lastTick).Milliseconds()
	m.lastTick = now

	// Collect accumulated events
	input := TickInput{
		DT:               dt,
		Timestamp:        now.UnixMilli(),
		ClientMessages:   m.clientMessages,
		IncomingWS:       m.incomingWS,
		ConnectionEvents: m.connectionEvents,
		Commands:         m.commands,
	}

	// Collect async responses
	if len(m.asyncResults) > 0 {
		input.Responses = m.asyncResults
		m.asyncResults = make(map[string]*AsyncResponse)
	}

	// Check timers
	var firedTimers []string
	for name, entry := range m.timers {
		if now.After(entry.nextFire) || now.Equal(entry.nextFire) {
			firedTimers = append(firedTimers, name)
			// Reset timer for next interval
			m.timers[name] = timerEntry{
				interval: entry.interval,
				nextFire: now.Add(entry.interval),
			}
		}
	}
	if len(firedTimers) > 0 {
		input.Timers = firedTimers
	}

	// Clear accumulated events
	m.clientMessages = nil
	m.incomingWS = nil
	m.connectionEvents = nil
	m.commands = nil
	m.mu.Unlock()

	// Serialize tick input
	data, err := m.Codec.Marshal(input)
	if err != nil {
		m.Logger.Error("marshal tick input failed", "module", m.Name, "error", err)
		return
	}

	// Call tick with timeout to prevent hung modules from blocking the loop
	start := time.Now()
	exitCode, _, err := m.callWithTimeout("tick", data, m.Config.TickTimeout)
	elapsed := time.Since(start)

	if err != nil {
		m.Logger.Error("tick call failed", "module", m.Name, "error", err)
		return
	}
	if exitCode != 0 {
		m.Logger.Error("tick returned error", "module", m.Name, "exit_code", exitCode)
	}

	// Budget monitoring
	budget := time.Duration(1000/m.tickRate) * time.Millisecond
	if elapsed > budget {
		m.Logger.Warn("tick exceeded budget",
			"module", m.Name,
			"elapsed", elapsed,
			"budget", budget,
		)
	}

	// Process any pending queries between ticks
	m.drainQueries()
}

// drainQueries processes queued queries/commands between ticks.
func (m *Module) drainQueries() {
	for {
		select {
		case req := <-m.queryCh:
			m.processQuery(req)
		default:
			return
		}
	}
}

// processQuery handles a query or command request.
func (m *Module) processQuery(req queryRequest) {
	// Determine if this is a command or query based on function existence
	funcName := "query"
	if m.Plugin.FunctionExists("command") && !m.Plugin.FunctionExists("query") {
		funcName = "command"
	}

	exitCode, output, err := m.Plugin.Call(funcName, req.data)
	if err != nil {
		req.result <- queryResponse{err: fmt.Errorf("%s call failed: %w", funcName, err)}
		return
	}
	if exitCode != 0 {
		req.result <- queryResponse{err: fmt.Errorf("%s returned exit code %d", funcName, exitCode)}
		return
	}

	req.result <- queryResponse{data: output}
}
