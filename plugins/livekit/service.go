package livekit

import (
	"github.com/livekit/protocol/auth"
)

// Service holds LiveKit SDK clients and credentials.
type Service struct {
	Room      RoomClient
	Egress    EgressClient
	Ingress   IngressClient
	APIKey    string
	APISecret string
}

// NewAuthProvider returns a key provider for webhook verification and token generation.
func (s *Service) NewAuthProvider() *auth.SimpleKeyProvider {
	return auth.NewSimpleKeyProvider(s.APIKey, s.APISecret)
}
