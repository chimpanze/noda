package livekit

import (
	"github.com/livekit/protocol/auth"
)

// Service holds LiveKit SDK clients and credentials. When the service config
// sets a "timeout" (a Go duration string, e.g. "5s"), Room/Egress/Ingress are
// wrapped by timeoutRoomClient/timeoutEgressClient/timeoutIngressClient so
// every API call is bounded by that per-call deadline; a deadline expiry
// produces an operation-scoped error via callWithTimeout (see timeout.go).
// Unset timeout leaves the bare SDK clients with no deadline.
type Service struct {
	Room      RoomClient
	Egress    EgressClient
	Ingress   IngressClient
	APIKey    string
	APISecret string
}

// newAuthProvider returns a key provider for webhook verification and token generation (test helper).
func (s *Service) newAuthProvider() *auth.SimpleKeyProvider {
	return auth.NewSimpleKeyProvider(s.APIKey, s.APISecret)
}
