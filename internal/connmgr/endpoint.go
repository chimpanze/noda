package connmgr

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
)

var _ api.ConnectionService = (*EndpointService)(nil)

// EndpointService wraps a Manager and implements api.ConnectionService.
// With a SyncBridge attached, sends also fan out to other instances; a
// publish failure fails the send (local delivery has already happened —
// callers wanting best-effort wire the node's error edge).
type EndpointService struct {
	manager  *Manager
	endpoint string
	bridge   *SyncBridge
}

// NewEndpointService creates a ConnectionService for the given manager. A nil
// bridge means local-only delivery (no cross-instance sync).
func NewEndpointService(manager *Manager, endpoint string, bridge *SyncBridge) *EndpointService {
	return &EndpointService{manager: manager, endpoint: endpoint, bridge: bridge}
}

func (s *EndpointService) Send(ctx context.Context, channel string, data any) error {
	if err := s.manager.Send(ctx, channel, data); err != nil {
		return err
	}
	if s.bridge == nil {
		return nil
	}
	payload, err := marshalDataString(data)
	if err != nil {
		return err
	}
	if err := s.bridge.Publish(ctx, s.endpoint, Envelope{Kind: "ws", Channel: channel, Payload: payload}); err != nil {
		return fmt.Errorf("cross-instance sync publish: %w", err)
	}
	return nil
}

func (s *EndpointService) SendSSE(ctx context.Context, channel string, event string, data any, id string) error {
	if err := s.manager.SendSSE(ctx, channel, event, data, id); err != nil {
		return err
	}
	if s.bridge == nil {
		return nil
	}
	payload, err := marshalDataString(data)
	if err != nil {
		return err
	}
	if err := s.bridge.Publish(ctx, s.endpoint, Envelope{Kind: "sse", Channel: channel, Payload: payload, Event: event, ID: id}); err != nil {
		return fmt.Errorf("cross-instance sync publish: %w", err)
	}
	return nil
}

// Manager returns the underlying connection manager.
func (s *EndpointService) Manager() *Manager {
	return s.manager
}
