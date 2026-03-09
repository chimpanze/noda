package connmgr

import "context"

// EndpointService wraps a Manager for a specific endpoint and implements api.ConnectionService.
type EndpointService struct {
	manager  *Manager
	endpoint string
}

// NewEndpointService creates a ConnectionService for a specific endpoint.
func NewEndpointService(manager *Manager, endpoint string) *EndpointService {
	return &EndpointService{manager: manager, endpoint: endpoint}
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
