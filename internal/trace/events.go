package trace

import (
	"encoding/json"
	"sync"
	"time"
)

// EventType identifies the kind of trace event.
type EventType string

const (
	EventWorkflowStarted   EventType = "workflow:started"
	EventWorkflowCompleted EventType = "workflow:completed"
	EventWorkflowFailed    EventType = "workflow:failed"
	EventNodeEntered       EventType = "node:entered"
	EventNodeCompleted     EventType = "node:completed"
	EventNodeFailed        EventType = "node:failed"
	EventEdgeFollowed      EventType = "edge:followed"
	EventRetryAttempted    EventType = "retry:attempted"
)

// Event is a single trace event emitted during execution.
type Event struct {
	Type       EventType `json:"type"`
	Timestamp  string    `json:"timestamp"`
	TraceID    string    `json:"trace_id"`
	WorkflowID string   `json:"workflow_id"`
	NodeID     string    `json:"node_id,omitempty"`
	NodeType   string    `json:"node_type,omitempty"`
	Output     string    `json:"output,omitempty"`
	Duration   string    `json:"duration,omitempty"`
	Error      string    `json:"error,omitempty"`
	FromNode   string    `json:"from_node,omitempty"`
	ToNode     string    `json:"to_node,omitempty"`
	Data       any       `json:"data,omitempty"`
}

// Subscriber receives trace events.
type Subscriber func(data []byte)

// EventHub broadcasts trace events to all connected subscribers.
type EventHub struct {
	mu          sync.RWMutex
	subscribers map[uint64]Subscriber
	nextID      uint64
}

// NewEventHub creates a new event hub.
func NewEventHub() *EventHub {
	return &EventHub{
		subscribers: make(map[uint64]Subscriber),
	}
}

// Subscribe registers a subscriber and returns an unsubscribe function.
func (h *EventHub) Subscribe(fn Subscriber) func() {
	h.mu.Lock()
	id := h.nextID
	h.nextID++
	h.subscribers[id] = fn
	h.mu.Unlock()

	return func() {
		h.mu.Lock()
		delete(h.subscribers, id)
		h.mu.Unlock()
	}
}

// Emit sends a trace event to all subscribers.
func (h *EventHub) Emit(event Event) {
	if event.Timestamp == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}

	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	h.mu.RLock()
	subs := make([]Subscriber, 0, len(h.subscribers))
	for _, fn := range h.subscribers {
		subs = append(subs, fn)
	}
	h.mu.RUnlock()

	for _, fn := range subs {
		fn(data)
	}
}

// SubscriberCount returns the number of active subscribers.
func (h *EventHub) SubscriberCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subscribers)
}
