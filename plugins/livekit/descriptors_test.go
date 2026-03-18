package livekit

import (
	"context"
	"fmt"
	"testing"

	lkproto "github.com/livekit/protocol/livekit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Descriptor metadata tests ---

func TestAllDescriptors_HaveNameAndDescription(t *testing.T) {
	p := &Plugin{}
	for _, reg := range p.Nodes() {
		d := reg.Descriptor
		assert.NotEmpty(t, d.Name(), "descriptor should have a name")
		assert.NotEmpty(t, d.Description(), "descriptor %s should have a description", d.Name())
	}
}

func TestAllDescriptors_HaveConfigSchema(t *testing.T) {
	p := &Plugin{}
	for _, reg := range p.Nodes() {
		d := reg.Descriptor
		schema := d.ConfigSchema()
		assert.NotNil(t, schema, "descriptor %s should have a config schema", d.Name())
		assert.Equal(t, "object", schema["type"], "descriptor %s schema should be an object", d.Name())
		assert.NotNil(t, schema["properties"], "descriptor %s schema should have properties", d.Name())
	}
}

func TestAllDescriptors_HaveOutputDescriptions(t *testing.T) {
	p := &Plugin{}
	for _, reg := range p.Nodes() {
		d := reg.Descriptor
		outputs := d.OutputDescriptions()
		assert.NotNil(t, outputs, "descriptor %s should have output descriptions", d.Name())
		assert.Contains(t, outputs, "success", "descriptor %s should have 'success' output", d.Name())
		assert.Contains(t, outputs, "error", "descriptor %s should have 'error' output", d.Name())
	}
}

// --- Factory and Outputs() tests ---

func TestAllFactories_ProduceValidExecutors(t *testing.T) {
	p := &Plugin{}
	for _, reg := range p.Nodes() {
		exec := reg.Factory(nil)
		assert.NotNil(t, exec, "factory for %s should produce a non-nil executor", reg.Descriptor.Name())
		outputs := exec.Outputs()
		assert.Contains(t, outputs, "success", "executor for %s should have 'success' output", reg.Descriptor.Name())
		assert.Contains(t, outputs, "error", "executor for %s should have 'error' output", reg.Descriptor.Name())
	}
}

// --- Missing service error tests for all node types ---

func TestAllNodes_MissingService(t *testing.T) {
	p := &Plugin{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	emptyServices := map[string]any{}

	// Each node needs different minimum config to get past field validation
	// to the service check. But service check happens first in all nodes.
	for _, reg := range p.Nodes() {
		exec := reg.Factory(nil)
		_, _, err := exec.Execute(context.Background(), nCtx,
			map[string]any{
				"identity":             "u",
				"room":                 "r",
				"name":                 "n",
				"track_sid":            "t",
				"muted":                true,
				"egress_id":            "e",
				"ingress_id":           "i",
				"input_type":           "rtmp",
				"participant_identity": "p",
				"data":                 "d",
				"output": map[string]any{
					"type":     "file",
					"filepath": "/tmp/test",
				},
			},
			emptyServices)
		require.Error(t, err, "node %s should error with missing service", reg.Descriptor.Name())
		assert.Contains(t, err.Error(), "service not configured", "node %s error should mention service", reg.Descriptor.Name())
	}
}

// --- Room Create optional parameters ---

func TestRoomCreateNode_WithOptionalParams(t *testing.T) {
	svc := testService()
	svc.Room = &mockRoomClient{
		createRoomFn: func(_ context.Context, req *lkproto.CreateRoomRequest) (*lkproto.Room, error) {
			return &lkproto.Room{
				Sid:             "RM_test",
				Name:            req.Name,
				EmptyTimeout:    req.EmptyTimeout,
				MaxParticipants: req.MaxParticipants,
				Metadata:        req.Metadata,
			}, nil
		},
	}
	exec := &roomCreateExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, data, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"name":             "my-room",
			"empty_timeout":    float64(300),
			"max_participants": float64(10),
			"metadata":         `{"key":"val"}`,
		},
		testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	result := data.(map[string]any)
	assert.Equal(t, "my-room", result["name"])
	assert.Equal(t, uint32(300), result["empty_timeout"])
	assert.Equal(t, uint32(10), result["max_participants"])
	assert.Equal(t, `{"key":"val"}`, result["metadata"])
}

// --- Room List with names filter ---

func TestRoomListNode_WithNamesFilter(t *testing.T) {
	svc := testService()
	svc.Room = &mockRoomClient{
		listRoomsFn: func(_ context.Context, req *lkproto.ListRoomsRequest) (*lkproto.ListRoomsResponse, error) {
			rooms := make([]*lkproto.Room, len(req.Names))
			for i, n := range req.Names {
				rooms[i] = &lkproto.Room{Name: n}
			}
			return &lkproto.ListRoomsResponse{Rooms: rooms}, nil
		},
	}
	exec := &roomListExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, data, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"names": []any{"room1", "room2"}},
		testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	rooms := data.(map[string]any)["rooms"].([]any)
	assert.Len(t, rooms, 2)
}

func TestRoomListNode_SDKError(t *testing.T) {
	svc := testService()
	svc.Room = &mockRoomClient{
		listRoomsFn: func(_ context.Context, _ *lkproto.ListRoomsRequest) (*lkproto.ListRoomsResponse, error) {
			return nil, fmt.Errorf("list failed")
		},
	}
	exec := &roomListExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{}, testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list failed")
}

// --- Room Delete error cases ---

func TestRoomDeleteNode_MissingRoom(t *testing.T) {
	svc := testService()
	exec := &roomDeleteExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{}, testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

func TestRoomDeleteNode_SDKError(t *testing.T) {
	svc := testService()
	svc.Room = &mockRoomClient{
		deleteRoomFn: func(_ context.Context, _ *lkproto.DeleteRoomRequest) (*lkproto.DeleteRoomResponse, error) {
			return nil, fmt.Errorf("delete failed")
		},
	}
	exec := &roomDeleteExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"room": "r"}, testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete failed")
}

// --- Room Update Metadata error cases ---

func TestRoomUpdateMetadataNode_MissingRoom(t *testing.T) {
	svc := testService()
	exec := &roomUpdateMetadataExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"metadata": "val"}, testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

func TestRoomUpdateMetadataNode_MissingMetadata(t *testing.T) {
	svc := testService()
	exec := &roomUpdateMetadataExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"room": "r"}, testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

func TestRoomUpdateMetadataNode_SDKError(t *testing.T) {
	svc := testService()
	svc.Room = &mockRoomClient{
		updateRoomMetadataFn: func(_ context.Context, _ *lkproto.UpdateRoomMetadataRequest) (*lkproto.Room, error) {
			return nil, fmt.Errorf("update failed")
		},
	}
	exec := &roomUpdateMetadataExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"room": "r", "metadata": "m"}, testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "update failed")
}

// --- SendData edge cases ---

func TestSendDataNode_MissingRoom(t *testing.T) {
	svc := testService()
	exec := &sendDataExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"data": "hello"}, testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

func TestSendDataNode_MissingData(t *testing.T) {
	svc := testService()
	exec := &sendDataExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"room": "r"}, testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

func TestSendDataNode_WithKindLossy(t *testing.T) {
	svc := testService()
	var captured *lkproto.SendDataRequest
	svc.Room = &mockRoomClient{
		sendDataFn: func(_ context.Context, req *lkproto.SendDataRequest) (*lkproto.SendDataResponse, error) {
			captured = req
			return &lkproto.SendDataResponse{}, nil
		},
	}
	exec := &sendDataExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"room": "r", "data": "hello", "kind": "lossy"},
		testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, lkproto.DataPacket_LOSSY, captured.Kind)
}

func TestSendDataNode_WithDestinationIdentities(t *testing.T) {
	svc := testService()
	var captured *lkproto.SendDataRequest
	svc.Room = &mockRoomClient{
		sendDataFn: func(_ context.Context, req *lkproto.SendDataRequest) (*lkproto.SendDataResponse, error) {
			captured = req
			return &lkproto.SendDataResponse{}, nil
		},
	}
	exec := &sendDataExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"room":                   "r",
			"data":                   "hello",
			"destination_identities": []any{"user1", "user2"},
		},
		testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, []string{"user1", "user2"}, captured.DestinationIdentities)
}

func TestSendDataNode_WithTopic(t *testing.T) {
	svc := testService()
	var captured *lkproto.SendDataRequest
	svc.Room = &mockRoomClient{
		sendDataFn: func(_ context.Context, req *lkproto.SendDataRequest) (*lkproto.SendDataResponse, error) {
			captured = req
			return &lkproto.SendDataResponse{}, nil
		},
	}
	exec := &sendDataExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"room": "r", "data": "hello", "topic": "chat"},
		testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	require.NotNil(t, captured.Topic)
	assert.Equal(t, "chat", *captured.Topic)
}

func TestSendDataNode_SDKError(t *testing.T) {
	svc := testService()
	svc.Room = &mockRoomClient{
		sendDataFn: func(_ context.Context, _ *lkproto.SendDataRequest) (*lkproto.SendDataResponse, error) {
			return nil, fmt.Errorf("send failed")
		},
	}
	exec := &sendDataExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"room": "r", "data": "hello"},
		testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "send failed")
}

// --- Participant List error cases ---

func TestParticipantListNode_MissingRoom(t *testing.T) {
	svc := testService()
	exec := &participantListExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{}, testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

func TestParticipantListNode_SDKError(t *testing.T) {
	svc := testService()
	svc.Room = &mockRoomClient{
		listParticipantsFn: func(_ context.Context, _ *lkproto.ListParticipantsRequest) (*lkproto.ListParticipantsResponse, error) {
			return nil, fmt.Errorf("list failed")
		},
	}
	exec := &participantListExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"room": "r"}, testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list failed")
}

// --- Participant Get error cases ---

func TestParticipantGetNode_MissingRoom(t *testing.T) {
	svc := testService()
	exec := &participantGetExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"identity": "u"}, testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

func TestParticipantGetNode_MissingIdentity(t *testing.T) {
	svc := testService()
	exec := &participantGetExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"room": "r"}, testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

// --- Participant Remove error cases ---

func TestParticipantRemoveNode_MissingRoom(t *testing.T) {
	svc := testService()
	exec := &participantRemoveExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"identity": "u"}, testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

func TestParticipantRemoveNode_SDKError(t *testing.T) {
	svc := testService()
	svc.Room = &mockRoomClient{
		removeParticipantFn: func(_ context.Context, _ *lkproto.RoomParticipantIdentity) (*lkproto.RemoveParticipantResponse, error) {
			return nil, fmt.Errorf("remove failed")
		},
	}
	exec := &participantRemoveExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"room": "r", "identity": "u"}, testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "remove failed")
}

// --- Participant Update error cases ---

func TestParticipantUpdateNode_MissingRoom(t *testing.T) {
	svc := testService()
	exec := &participantUpdateExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"identity": "u"}, testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

func TestParticipantUpdateNode_SDKError(t *testing.T) {
	svc := testService()
	svc.Room = &mockRoomClient{
		updateParticipantFn: func(_ context.Context, _ *lkproto.UpdateParticipantRequest) (*lkproto.ParticipantInfo, error) {
			return nil, fmt.Errorf("update failed")
		},
	}
	exec := &participantUpdateExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"room": "r", "identity": "u"},
		testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "update failed")
}

func TestParticipantUpdateNode_WithAllPermissions(t *testing.T) {
	svc := testService()
	var captured *lkproto.UpdateParticipantRequest
	svc.Room = &mockRoomClient{
		updateParticipantFn: func(_ context.Context, req *lkproto.UpdateParticipantRequest) (*lkproto.ParticipantInfo, error) {
			captured = req
			return &lkproto.ParticipantInfo{Sid: "PA_1", Identity: req.Identity}, nil
		},
	}
	exec := &participantUpdateExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"room":     "r",
			"identity": "u",
			"permissions": map[string]any{
				"canPublish":     true,
				"canSubscribe":   false,
				"canPublishData": true,
				"hidden":         true,
				"recorder":       true,
			},
		},
		testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	require.NotNil(t, captured.Permission)
	assert.True(t, captured.Permission.CanPublish)
	assert.False(t, captured.Permission.CanSubscribe)
	assert.True(t, captured.Permission.CanPublishData)
	assert.True(t, captured.Permission.Hidden)
}

// --- MuteTrack error cases ---

func TestMuteTrackNode_MissingRoom(t *testing.T) {
	svc := testService()
	exec := &muteTrackExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"identity": "u", "track_sid": "t", "muted": true},
		testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

func TestMuteTrackNode_MutedNotBool(t *testing.T) {
	svc := testService()
	exec := &muteTrackExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"room": "r", "identity": "u", "track_sid": "t", "muted": "yes"},
		testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a boolean")
}

func TestMuteTrackNode_SDKError(t *testing.T) {
	svc := testService()
	svc.Room = &mockRoomClient{
		mutePublishedTrackFn: func(_ context.Context, _ *lkproto.MuteRoomTrackRequest) (*lkproto.MuteRoomTrackResponse, error) {
			return nil, fmt.Errorf("mute failed")
		},
	}
	exec := &muteTrackExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"room": "r", "identity": "u", "track_sid": "t", "muted": true},
		testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mute failed")
}

func TestMuteTrackNode_NilTrackInResponse(t *testing.T) {
	svc := testService()
	svc.Room = &mockRoomClient{
		mutePublishedTrackFn: func(_ context.Context, _ *lkproto.MuteRoomTrackRequest) (*lkproto.MuteRoomTrackResponse, error) {
			return &lkproto.MuteRoomTrackResponse{Track: nil}, nil
		},
	}
	exec := &muteTrackExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, data, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"room": "r", "identity": "u", "track_sid": "t", "muted": false},
		testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	result := data.(map[string]any)
	assert.Equal(t, false, result["muted"])
	// track_sid should not be present when Track is nil
	_, hasSid := result["track_sid"]
	assert.False(t, hasSid)
}

// --- Egress Start Room Composite edge cases ---

func TestEgressStartRoomCompositeNode_WithLayoutAndAudioOnly(t *testing.T) {
	svc := testService()
	var captured *lkproto.RoomCompositeEgressRequest
	svc.Egress = &mockEgressClient{
		startRoomCompositeEgressFn: func(_ context.Context, req *lkproto.RoomCompositeEgressRequest) (*lkproto.EgressInfo, error) {
			captured = req
			return &lkproto.EgressInfo{EgressId: "EG_1", RoomName: req.RoomName, Status: lkproto.EgressStatus_EGRESS_STARTING}, nil
		},
	}
	exec := &egressStartRoomCompositeExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"room":       "my-room",
			"layout":     "grid-dark",
			"audio_only": true,
			"output": map[string]any{
				"type":     "file",
				"filepath": "/tmp/test.mp4",
			},
		},
		testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, "grid-dark", captured.Layout)
	assert.True(t, captured.AudioOnly)
}

func TestEgressStartRoomCompositeNode_SDKError(t *testing.T) {
	svc := testService()
	svc.Egress = &mockEgressClient{
		startRoomCompositeEgressFn: func(_ context.Context, _ *lkproto.RoomCompositeEgressRequest) (*lkproto.EgressInfo, error) {
			return nil, fmt.Errorf("egress failed")
		},
	}
	exec := &egressStartRoomCompositeExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"room":   "r",
			"output": map[string]any{"type": "file", "filepath": "/tmp/t"},
		},
		testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "egress failed")
}

func TestEgressStartRoomCompositeNode_MissingOutput(t *testing.T) {
	svc := testService()
	exec := &egressStartRoomCompositeExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"room": "r"},
		testServices(svc))
	require.Error(t, err)
}

// --- Egress Start Track edge cases ---

func TestEgressStartTrackNode_MissingRoom(t *testing.T) {
	svc := testService()
	exec := &egressStartTrackExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"track_sid": "t",
			"output":    map[string]any{"type": "file", "filepath": "/tmp/t"},
		},
		testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

func TestEgressStartTrackNode_MissingTrackSID(t *testing.T) {
	svc := testService()
	exec := &egressStartTrackExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"room":   "r",
			"output": map[string]any{"type": "file", "filepath": "/tmp/t"},
		},
		testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

func TestEgressStartTrackNode_WithS3Output(t *testing.T) {
	svc := testService()
	exec := &egressStartTrackExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, data, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"room":      "my-room",
			"track_sid": "TR_abc",
			"output": map[string]any{
				"type":     "s3",
				"filepath": "/tmp/track.ogg",
				"bucket":   "my-bucket",
			},
		},
		testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, "EG_2", data.(map[string]any)["egress_id"])
}

func TestEgressStartTrackNode_WithGCSOutput(t *testing.T) {
	svc := testService()
	exec := &egressStartTrackExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"room":      "my-room",
			"track_sid": "TR_abc",
			"output": map[string]any{
				"type":        "gcs",
				"filepath":    "/tmp/track.ogg",
				"bucket":      "gcs-bucket",
				"credentials": "cred",
			},
		},
		testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
}

func TestEgressStartTrackNode_WithAzureOutput(t *testing.T) {
	svc := testService()
	exec := &egressStartTrackExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"room":      "my-room",
			"track_sid": "TR_abc",
			"output": map[string]any{
				"type":           "azure",
				"filepath":       "/tmp/track.ogg",
				"account_name":   "acc",
				"account_key":    "key",
				"container_name": "cont",
			},
		},
		testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
}

func TestEgressStartTrackNode_SDKError(t *testing.T) {
	svc := testService()
	svc.Egress = &mockEgressClient{
		startTrackEgressFn: func(_ context.Context, _ *lkproto.TrackEgressRequest) (*lkproto.EgressInfo, error) {
			return nil, fmt.Errorf("track egress failed")
		},
	}
	exec := &egressStartTrackExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"room":      "r",
			"track_sid": "t",
			"output":    map[string]any{"type": "file", "filepath": "/tmp/t"},
		},
		testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "track egress failed")
}

// --- Egress Stop error cases ---

func TestEgressStopNode_MissingEgressID(t *testing.T) {
	svc := testService()
	exec := &egressStopExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{}, testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

func TestEgressStopNode_SDKError(t *testing.T) {
	svc := testService()
	svc.Egress = &mockEgressClient{
		stopEgressFn: func(_ context.Context, _ *lkproto.StopEgressRequest) (*lkproto.EgressInfo, error) {
			return nil, fmt.Errorf("stop failed")
		},
	}
	exec := &egressStopExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"egress_id": "EG_1"}, testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stop failed")
}

// --- Egress List edge cases ---

func TestEgressListNode_WithRoomFilter(t *testing.T) {
	svc := testService()
	var captured *lkproto.ListEgressRequest
	svc.Egress = &mockEgressClient{
		listEgressFn: func(_ context.Context, req *lkproto.ListEgressRequest) (*lkproto.ListEgressResponse, error) {
			captured = req
			return &lkproto.ListEgressResponse{Items: []*lkproto.EgressInfo{}}, nil
		},
	}
	exec := &egressListExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, data, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"room": "my-room"}, testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, "my-room", captured.RoomName)
	items := data.(map[string]any)["items"].([]any)
	assert.Len(t, items, 0)
}

func TestEgressListNode_SDKError(t *testing.T) {
	svc := testService()
	svc.Egress = &mockEgressClient{
		listEgressFn: func(_ context.Context, _ *lkproto.ListEgressRequest) (*lkproto.ListEgressResponse, error) {
			return nil, fmt.Errorf("list failed")
		},
	}
	exec := &egressListExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{}, testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list failed")
}

// --- Ingress Create edge cases ---

func TestIngressCreateNode_WHIPInputType(t *testing.T) {
	svc := testService()
	exec := &ingressCreateExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, data, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"input_type":           "whip",
			"room":                 "my-room",
			"participant_identity": "streamer",
		},
		testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, "IN_1", data.(map[string]any)["ingress_id"])
}

func TestIngressCreateNode_URLInputType(t *testing.T) {
	svc := testService()
	exec := &ingressCreateExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"input_type":           "url",
			"room":                 "my-room",
			"participant_identity": "streamer",
			"url":                  "https://example.com/stream.mp4",
		},
		testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
}

func TestIngressCreateNode_WithParticipantName(t *testing.T) {
	svc := testService()
	var captured *lkproto.CreateIngressRequest
	svc.Ingress = &mockIngressClient{
		createIngressFn: func(_ context.Context, req *lkproto.CreateIngressRequest) (*lkproto.IngressInfo, error) {
			captured = req
			return &lkproto.IngressInfo{
				IngressId:           "IN_1",
				RoomName:            req.RoomName,
				ParticipantIdentity: req.ParticipantIdentity,
				ParticipantName:     req.ParticipantName,
				InputType:           req.InputType,
			}, nil
		},
	}
	exec := &ingressCreateExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, data, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"input_type":           "rtmp",
			"room":                 "my-room",
			"participant_identity": "streamer",
			"participant_name":     "Streamer Name",
		},
		testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, "Streamer Name", captured.ParticipantName)
	assert.Equal(t, "Streamer Name", data.(map[string]any)["participant_name"])
}

func TestIngressCreateNode_MissingInputType(t *testing.T) {
	svc := testService()
	exec := &ingressCreateExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"room":                 "r",
			"participant_identity": "p",
		},
		testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

func TestIngressCreateNode_MissingRoom(t *testing.T) {
	svc := testService()
	exec := &ingressCreateExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"input_type":           "rtmp",
			"participant_identity": "p",
		},
		testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

func TestIngressCreateNode_MissingParticipantIdentity(t *testing.T) {
	svc := testService()
	exec := &ingressCreateExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"input_type": "rtmp",
			"room":       "r",
		},
		testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

func TestIngressCreateNode_SDKError(t *testing.T) {
	svc := testService()
	svc.Ingress = &mockIngressClient{
		createIngressFn: func(_ context.Context, _ *lkproto.CreateIngressRequest) (*lkproto.IngressInfo, error) {
			return nil, fmt.Errorf("ingress create failed")
		},
	}
	exec := &ingressCreateExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"input_type":           "rtmp",
			"room":                 "r",
			"participant_identity": "p",
		},
		testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ingress create failed")
}

// --- Ingress List edge cases ---

func TestIngressListNode_WithRoomFilter(t *testing.T) {
	svc := testService()
	var captured *lkproto.ListIngressRequest
	svc.Ingress = &mockIngressClient{
		listIngressFn: func(_ context.Context, req *lkproto.ListIngressRequest) (*lkproto.ListIngressResponse, error) {
			captured = req
			return &lkproto.ListIngressResponse{Items: []*lkproto.IngressInfo{}}, nil
		},
	}
	exec := &ingressListExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"room": "my-room"}, testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, "my-room", captured.RoomName)
}

func TestIngressListNode_SDKError(t *testing.T) {
	svc := testService()
	svc.Ingress = &mockIngressClient{
		listIngressFn: func(_ context.Context, _ *lkproto.ListIngressRequest) (*lkproto.ListIngressResponse, error) {
			return nil, fmt.Errorf("list failed")
		},
	}
	exec := &ingressListExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{}, testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list failed")
}

// --- Ingress Delete error cases ---

func TestIngressDeleteNode_MissingIngressID(t *testing.T) {
	svc := testService()
	exec := &ingressDeleteExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{}, testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

func TestIngressDeleteNode_SDKError(t *testing.T) {
	svc := testService()
	svc.Ingress = &mockIngressClient{
		deleteIngressFn: func(_ context.Context, _ *lkproto.DeleteIngressRequest) (*lkproto.IngressInfo, error) {
			return nil, fmt.Errorf("delete failed")
		},
	}
	exec := &ingressDeleteExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"ingress_id": "IN_1"}, testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete failed")
}

// --- Token node edge cases ---

func TestTokenNode_WithNameAndMetadata(t *testing.T) {
	svc := testService()
	exec := &tokenExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, data, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"identity": "user1",
			"room":     "room1",
			"name":     "User One",
			"metadata": `{"role":"admin"}`,
		},
		testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.NotEmpty(t, data.(map[string]any)["token"])
}

func TestTokenNode_WithCustomTTL(t *testing.T) {
	svc := testService()
	exec := &tokenExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"identity": "user1",
			"room":     "room1",
			"ttl":      "1h",
		},
		testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
}

func TestTokenNode_MissingRoom(t *testing.T) {
	svc := testService()
	exec := &tokenExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"identity": "u"},
		testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

// --- Plugin.CreateService success ---

func TestPlugin_CreateService_Success(t *testing.T) {
	p := &Plugin{}
	svc, err := p.CreateService(map[string]any{
		"url":        "wss://example.livekit.cloud",
		"api_key":    "key",
		"api_secret": "secret",
	})
	require.NoError(t, err)
	require.NotNil(t, svc)

	s := svc.(*Service)
	assert.Equal(t, "key", s.APIKey)
	assert.Equal(t, "secret", s.APISecret)
	assert.NotNil(t, s.Room)
	assert.NotNil(t, s.Egress)
	assert.NotNil(t, s.Ingress)
}
