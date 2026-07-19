package livekit

import (
	"testing"

	"github.com/chimpanze/noda/internal/registry"
	"github.com/stretchr/testify/assert"
)

func TestConfigSchemasMatchExecutors(t *testing.T) {
	tests := []struct {
		nodeType     string
		schema       map[string]any
		minimalValid map[string]any // smallest config the executor accepts (from docs example)
		emptyValid   bool           // does the executor run with config {}?
		invalid      map[string]any // one config the executor would reject/misuse
	}{
		{"lk.token", (&tokenDescriptor{}).ConfigSchema(),
			map[string]any{"identity": "usr_1", "room": "room-1"}, false,
			map[string]any{"room": "room-1"}},

		{"lk.room_create", (&roomCreateDescriptor{}).ConfigSchema(),
			map[string]any{"name": "room-1"}, false,
			map[string]any{"name": true}},

		{"lk.room_list", (&roomListDescriptor{}).ConfigSchema(),
			map[string]any{}, true,
			map[string]any{"names": "not-an-array"}},

		{"lk.room_delete", (&roomDeleteDescriptor{}).ConfigSchema(),
			map[string]any{"room": "room-1"}, false,
			map[string]any{"room": 42}},

		{"lk.room_update_metadata", (&roomUpdateMetadataDescriptor{}).ConfigSchema(),
			map[string]any{"room": "room-1", "metadata": "m"}, false,
			map[string]any{"room": "room-1"}},

		{"lk.send_data", (&sendDataDescriptor{}).ConfigSchema(),
			map[string]any{"room": "room-1", "data": "hello"}, false,
			map[string]any{"data": "hello"}},

		{"lk.participant_list", (&participantListDescriptor{}).ConfigSchema(),
			map[string]any{"room": "room-1"}, false,
			map[string]any{"room": 42}},

		{"lk.participant_get", (&participantGetDescriptor{}).ConfigSchema(),
			map[string]any{"room": "room-1", "identity": "usr_1"}, false,
			map[string]any{"room": "room-1"}},

		{"lk.participant_remove", (&participantRemoveDescriptor{}).ConfigSchema(),
			map[string]any{"room": "room-1", "identity": "usr_1"}, false,
			map[string]any{"room": "room-1"}},

		{"lk.participant_update", (&participantUpdateDescriptor{}).ConfigSchema(),
			map[string]any{"room": "room-1", "identity": "usr_1"}, false,
			map[string]any{"identity": "usr_1"}},

		{"lk.mute_track", (&muteTrackDescriptor{}).ConfigSchema(),
			map[string]any{"room": "room-1", "identity": "usr_1", "track_sid": "TR_1", "muted": true}, false,
			map[string]any{"room": "room-1", "identity": "usr_1", "track_sid": "TR_1"}},

		{"lk.egress_start_room_composite", (&egressStartRoomCompositeDescriptor{}).ConfigSchema(),
			map[string]any{"room": "room-1", "output": map[string]any{"type": "file", "filepath": "/tmp/out.mp4"}}, false,
			map[string]any{"room": "room-1"}},

		{"lk.egress_start_track", (&egressStartTrackDescriptor{}).ConfigSchema(),
			map[string]any{"room": "room-1", "track_sid": "TR_1", "output": map[string]any{"filepath": "/tmp/out.mp4"}}, false,
			map[string]any{"room": "room-1", "track_sid": "TR_1"}},

		{"lk.egress_stop", (&egressStopDescriptor{}).ConfigSchema(),
			map[string]any{"egress_id": "EG_1"}, false,
			map[string]any{"egress_id": 42}},

		{"lk.egress_list", (&egressListDescriptor{}).ConfigSchema(),
			map[string]any{}, true,
			map[string]any{"room": 123}},

		{"lk.ingress_create", (&ingressCreateDescriptor{}).ConfigSchema(),
			map[string]any{"input_type": "rtmp", "room": "room-1", "participant_identity": "usr_1"}, false,
			map[string]any{"room": "room-1", "participant_identity": "usr_1"}},

		{"lk.ingress_list", (&ingressListDescriptor{}).ConfigSchema(),
			map[string]any{}, true,
			map[string]any{"room": true}},

		{"lk.ingress_delete", (&ingressDeleteDescriptor{}).ConfigSchema(),
			map[string]any{"ingress_id": "IN_1"}, false,
			map[string]any{"ingress_id": 42}},
	}
	for _, tt := range tests {
		t.Run(tt.nodeType, func(t *testing.T) {
			assert.Empty(t, registry.CheckSchemaVocabulary(tt.schema))
			assert.Empty(t, registry.ValidateNodeConfig(tt.schema, tt.minimalValid), "minimal valid config must pass")
			emptyErrs := registry.ValidateNodeConfig(tt.schema, map[string]any{})
			if tt.emptyValid {
				assert.Empty(t, emptyErrs, "executor accepts {}, schema must too")
			} else {
				assert.NotEmpty(t, emptyErrs, "executor rejects {}, schema must too")
			}
			assert.NotEmpty(t, registry.ValidateNodeConfig(tt.schema, tt.invalid))
		})
	}
}
