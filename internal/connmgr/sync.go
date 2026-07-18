package connmgr

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/chimpanze/noda/pkg/api"
)

// syncChannelPrefix namespaces cross-instance sync traffic in the pubsub service.
const syncChannelPrefix = "noda:sync:"

// Envelope is the versioned cross-instance sync message. Payload carries the
// pre-marshaled bytes the local manager delivered, so every instance emits
// byte-identical frames and no JSON round-trip mangling occurs.
type Envelope struct {
	V        int    `json:"v"`
	Instance string `json:"instance"`
	Kind     string `json:"kind"` // "ws" or "sse"
	Channel  string `json:"channel"`
	Payload  string `json:"payload"`
	Event    string `json:"event,omitempty"` // SSE only
	ID       string `json:"id,omitempty"`    // SSE only
}

// SyncBridge fans ConnectionService sends out to other instances through a
// pubsub service and feeds remote envelopes into the local Manager (#363).
type SyncBridge struct {
	pubsub     api.PubSubService
	instanceID string
	logger     *slog.Logger
	backoff    time.Duration // subscribe retry delay; shortened in tests
}

func NewSyncBridge(pubsub api.PubSubService, instanceID string, logger *slog.Logger) *SyncBridge {
	if logger == nil {
		logger = slog.Default()
	}
	return &SyncBridge{pubsub: pubsub, instanceID: instanceID, logger: logger, backoff: time.Second}
}

// Publish sends an envelope to the endpoint's sync channel. Errors surface to
// the caller: with sync configured, a lost publish means remote users silently
// miss messages, so the sending node fails loudly.
func (b *SyncBridge) Publish(ctx context.Context, endpoint string, env Envelope) error {
	env.V = 1
	env.Instance = b.instanceID
	return b.pubsub.Publish(ctx, syncChannelPrefix+endpoint, env)
}

// Run subscribes to the endpoint's sync channel and delivers remote envelopes
// to mgr until ctx is cancelled, reconnecting with backoff on subscribe errors.
func (b *SyncBridge) Run(ctx context.Context, endpoint string, mgr *Manager) {
	channel := syncChannelPrefix + endpoint
	for {
		err := b.pubsub.Subscribe(ctx, channel, func(payload any) error {
			b.deliver(ctx, endpoint, mgr, payload)
			return nil // a bad message must never kill the subscription
		})
		if ctx.Err() != nil {
			return
		}
		b.logger.Warn("connection sync: subscribe failed; retrying",
			"endpoint", endpoint, "error", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(b.backoff):
		}
	}
}

func (b *SyncBridge) deliver(ctx context.Context, endpoint string, mgr *Manager, payload any) {
	raw, err := json.Marshal(payload) // pubsub hands us a decoded map; round-trip into the struct
	if err != nil {
		b.logger.Warn("connection sync: envelope marshal failed", "endpoint", endpoint, "error", err)
		return
	}
	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		b.logger.Warn("connection sync: malformed envelope dropped", "endpoint", endpoint, "error", err)
		return
	}
	if env.Instance == b.instanceID {
		return // Redis echoes to the publisher; local delivery already happened
	}
	if env.V != 1 {
		b.logger.Warn("connection sync: unknown envelope version dropped", "endpoint", endpoint, "v", env.V)
		return
	}
	switch env.Kind {
	case "ws":
		if err := mgr.Send(ctx, env.Channel, env.Payload); err != nil {
			b.logger.Debug("connection sync: remote ws delivery failed", "channel", env.Channel, "error", err)
		}
	case "sse":
		if err := mgr.SendSSE(ctx, env.Channel, env.Event, env.Payload, env.ID); err != nil {
			b.logger.Debug("connection sync: remote sse delivery failed", "channel", env.Channel, "error", err)
		}
	default:
		b.logger.Warn("connection sync: unknown envelope kind dropped", "endpoint", endpoint, "kind", env.Kind)
	}
}
