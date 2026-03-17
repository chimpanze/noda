package livekit

import (
	"github.com/livekit/protocol/auth"
	lkproto "github.com/livekit/protocol/livekit"
)

const serviceDep = "livekit"

// applyGrants maps grant keys from a config map to a VideoGrant.
func applyGrants(grants map[string]any, vg *auth.VideoGrant) {
	if v, ok := grants["roomJoin"].(bool); ok {
		vg.RoomJoin = v
	}
	if v, ok := grants["roomCreate"].(bool); ok {
		vg.RoomCreate = v
	}
	if v, ok := grants["roomList"].(bool); ok {
		vg.RoomList = v
	}
	if v, ok := grants["roomAdmin"].(bool); ok {
		vg.RoomAdmin = v
	}
	if v, ok := grants["canPublish"].(bool); ok {
		vg.SetCanPublish(v)
	}
	if v, ok := grants["canSubscribe"].(bool); ok {
		vg.SetCanSubscribe(v)
	}
	if v, ok := grants["canPublishData"].(bool); ok {
		vg.SetCanPublishData(v)
	}
	if v, ok := grants["canUpdateOwnMetadata"].(bool); ok {
		vg.SetCanUpdateOwnMetadata(v)
	}
	if v, ok := grants["hidden"].(bool); ok {
		vg.Hidden = v
	}
	if v, ok := grants["recorder"].(bool); ok {
		vg.Recorder = v
	}
	if v, ok := grants["canPublishSources"].([]any); ok {
		sources := make([]lkproto.TrackSource, 0, len(v))
		for _, src := range v {
			if s, ok := src.(string); ok {
				if val, exists := lkproto.TrackSource_value[s]; exists {
					sources = append(sources, lkproto.TrackSource(val))
				}
			}
		}
		vg.SetCanPublishSources(sources)
	}
}
