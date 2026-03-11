package noda

// WSConnect opens a managed WebSocket connection on the host.
func WSConnect(id, url string, headers map[string]string) error {
	payload := map[string]any{
		"id":  id,
		"url": url,
	}
	if len(headers) > 0 {
		payload["headers"] = headers
	}
	_, err := Call("", "ws_connect", payload)
	return err
}

// WSSend sends data over a managed WebSocket connection.
func WSSend(id string, data any) error {
	_, err := Call("", "ws_send", map[string]any{
		"id":   id,
		"data": data,
	})
	return err
}

// WSClose closes a managed WebSocket connection.
func WSClose(id string, code int, reason string) error {
	_, err := Call("", "ws_close", map[string]any{
		"id":     id,
		"code":   code,
		"reason": reason,
	})
	return err
}

// WSConfigure configures a managed WebSocket connection (e.g., heartbeat settings).
func WSConfigure(id string, heartbeatIntervalMs float64, heartbeatPayload any) error {
	_, err := Call("", "ws_configure", map[string]any{
		"id":                 id,
		"heartbeat_interval": heartbeatIntervalMs,
		"heartbeat_payload":  heartbeatPayload,
	})
	return err
}
