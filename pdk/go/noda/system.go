package noda

// Log sends a log message to the Noda host.
func Log(level, message string, fields map[string]any) {
	Call("", "log", map[string]any{
		"level":   level,
		"message": message,
		"fields":  fields,
	})
}

// LogDebug logs a debug-level message.
func LogDebug(message string, fields map[string]any) { Log("debug", message, fields) }

// LogInfo logs an info-level message.
func LogInfo(message string, fields map[string]any) { Log("info", message, fields) }

// LogWarn logs a warn-level message.
func LogWarn(message string, fields map[string]any) { Log("warn", message, fields) }

// LogError logs an error-level message.
func LogError(message string, fields map[string]any) { Log("error", message, fields) }

// TriggerWorkflow triggers a workflow execution on the host.
func TriggerWorkflow(workflow string, input map[string]any) error {
	_, err := Call("", "trigger_workflow", map[string]any{
		"workflow": workflow,
		"input":    input,
	})
	return err
}

// SetTimer creates or updates a named timer that fires at the given interval.
// When the timer fires, its name appears in TickInput.Timers.
func SetTimer(name string, intervalMs int64) error {
	_, err := Call("", "set_timer", map[string]any{
		"name":        name,
		"interval_ms": intervalMs,
	})
	return err
}

// ClearTimer removes a named timer.
func ClearTimer(name string) error {
	_, err := Call("", "clear_timer", map[string]any{
		"name": name,
	})
	return err
}
