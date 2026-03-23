package connmgr

import (
	"context"

	"github.com/chimpanze/noda/pkg/api"
)

var _ api.ConnectionService = (*EndpointService)(nil)

// EndpointService wraps a Manager and implements api.ConnectionService.
type EndpointService struct {
	manager *Manager
}

// NewEndpointService creates a ConnectionService for the given manager.
func NewEndpointService(manager *Manager, _ string) *EndpointService {
	return &EndpointService{manager: manager}
}

func (s *EndpointService) Send(ctx context.Context, channel string, data any) error {
	return s.manager.Send(ctx, channel, data)
}

func (s *EndpointService) SendSSE(ctx context.Context, channel string, event string, data any, id string) error {
	return s.manager.SendSSE(ctx, channel, event, data, id)
}

// Manager returns the underlying connection manager.
func (s *EndpointService) Manager() *Manager {
	return s.manager
}
