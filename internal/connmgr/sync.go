package connmgr

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"time"
	"unicode/utf8"

	"github.com/chimpanze/noda/pkg/api"
)

// syncChannelPrefix namespaces cross-instance sync traffic in the pubsub service.
const syncChannelPrefix = "noda:sync:"

// Envelope is the versioned cross-instance sync message (current version:
// 2, the only version accepted). Payload carries the pre-marshaled bytes
// the local manager delivered, so every instance emits byte-identical
// frames and no JSON round-trip mangling occurs.
//
// Enc selects the payload encoding: "" means Payload is the raw string
// (valid UTF-8), "b64" means Payload is the base64 (StdEncoding) of the
// raw bytes — chosen automatically for non-UTF-8 payloads, which
// json.Marshal would otherwise silently mangle into U+FFFD (#372).
type Envelope struct {
	V        int    `json:"v"`
	Instance string `json:"instance"`
	Kind     string `json:"kind"` // "ws" or "sse"
	Channel  string `json:"channel"`
	Payload  string `json:"payload"`
	Enc      string `json:"enc,omitempty"`   // "" = plain string; "b64" = base64-encoded bytes
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
	env.V = 2
	if !utf8.ValidString(env.Payload) {
		// Non-UTF-8 payloads would be silently mangled by json.Marshal
		// (invalid bytes → U+FFFD); ship them base64-encoded instead.
		// UTF-8 payloads stay plain to avoid the base64 size overhead (#372).
		env.Enc = "b64"
		env.Payload = base64.StdEncoding.EncodeToString([]byte(env.Payload))
	}
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
	if env.V != 2 {
		b.logger.Warn("connection sync: unknown envelope version dropped", "endpoint", endpoint, "v", env.V)
		return
	}
	// marshalData/marshalDataString pass []byte through untouched, so a
	// decoded b64 payload is delivered byte-exact (#372).
	var data any = env.Payload
	if env.Enc == "b64" {
		raw, err := base64.StdEncoding.DecodeString(env.Payload)
		if err != nil {
			b.logger.Warn("connection sync: undecodable b64 payload dropped", "endpoint", endpoint, "error", err)
			return
		}
		data = raw
	}
	switch env.Kind {
	case "ws":
		if err := mgr.Send(ctx, env.Channel, data); err != nil {
			b.logger.Debug("connection sync: remote ws delivery failed", "channel", env.Channel, "error", err)
		}
	case "sse":
		if err := mgr.SendSSE(ctx, env.Channel, env.Event, data, env.ID); err != nil {
			b.logger.Debug("connection sync: remote sse delivery failed", "channel", env.Channel, "error", err)
		}
	default:
		b.logger.Warn("connection sync: unknown envelope kind dropped", "endpoint", endpoint, "kind", env.Kind)
	}
}
