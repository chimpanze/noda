package trace

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/chimpanze/noda/internal/bounded"
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
	WorkflowID string    `json:"workflow_id"`
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

// traceInboxCapacity is the bounded inbox size per subscriber.
// Events are small (~few hundred bytes) and subscribers are few
// (dev-mode editor websocket clients). 256 is generous.
const traceInboxCapacity = 256

type subscriberSlot struct {
	fn     Subscriber
	inbox  *bounded.Queue[[]byte]
	cancel context.CancelFunc
	done   chan struct{}
}

// EventHub broadcasts trace events to all connected subscribers via
// per-subscriber goroutines and bounded inboxes (DropOldest). Emit is
// non-blocking; slow subscribers see drops in their own inbox without
// affecting others.
type EventHub struct {
	mu          sync.RWMutex
	subscribers map[uint64]*subscriberSlot
	nextID      uint64
}

// NewEventHub creates a new event hub.
func NewEventHub() *EventHub {
	return &EventHub{
		subscribers: make(map[uint64]*subscriberSlot),
	}
}

// Subscribe registers a subscriber and returns an unsubscribe function.
// The unsubscribe function blocks until the subscriber goroutine has
// fully exited.
func (h *EventHub) Subscribe(fn Subscriber) func() {
	inbox := bounded.New[[]byte](traceInboxCapacity, bounded.DropOldest)
	ctx, cancel := context.WithCancel(context.Background())
	slot := &subscriberSlot{
		fn:     fn,
		inbox:  inbox,
		cancel: cancel,
		done:   make(chan struct{}),
	}

	h.mu.Lock()
	id := h.nextID
	h.nextID++
	h.subscribers[id] = slot
	h.mu.Unlock()

	go func() {
		defer close(slot.done)
		for {
			data, ok := slot.inbox.Pop(ctx)
			if !ok {
				return
			}
			fn(data)
		}
	}()

	return func() {
		h.mu.Lock()
		delete(h.subscribers, id)
		h.mu.Unlock()
		slot.cancel()
		slot.inbox.Close()
		<-slot.done
	}
}

// Emit sends a trace event to all subscribers. Non-blocking — events are
// enqueued to per-subscriber bounded inboxes.
func (h *EventHub) Emit(event Event) {
	if event.Timestamp == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if m, ok := event.Data.(map[string]any); ok {
		event.Data = redactSecrets(m)
	}
	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	h.mu.RLock()
	subs := make([]*subscriberSlot, 0, len(h.subscribers))
	for _, s := range h.subscribers {
		subs = append(subs, s)
	}
	h.mu.RUnlock()

	for _, s := range subs {
		s.inbox.Push(data)
	}
}

// subscriberCount returns the number of active subscribers (test helper).
func (h *EventHub) subscriberCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subscribers)
}
