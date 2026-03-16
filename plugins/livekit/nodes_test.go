package livekit

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/chimpanze/noda/pkg/api"
	lkproto "github.com/livekit/protocol/livekit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock execution context ---

type mockExecCtx struct {
	resolveFunc func(expr string) (any, error)
}

func (m *mockExecCtx) Input() any          { return nil }
func (m *mockExecCtx) Auth() *api.AuthData { return nil }
func (m *mockExecCtx) Trigger() api.TriggerData {
	return api.TriggerData{Type: "test", Timestamp: time.Now(), TraceID: "test-trace"}
}
func (m *mockExecCtx) Resolve(expr string) (any, error) {
	if m.resolveFunc != nil {
		return m.resolveFunc(expr)
	}
	return expr, nil
}
func (m *mockExecCtx) ResolveWithVars(expr string, _ map[string]any) (any, error) {
	return m.Resolve(expr)
}
func (m *mockExecCtx) Log(_ string, _ string, _ map[string]any) {}

func identityResolve(expr string) (any, error) {
	return expr, nil
}

// --- Mock Room Client ---

type mockRoomClient struct {
	createRoomFn          func(ctx context.Context, req *lkproto.CreateRoomRequest) (*lkproto.Room, error)
	listRoomsFn           func(ctx context.Context, req *lkproto.ListRoomsRequest) (*lkproto.ListRoomsResponse, error)
	deleteRoomFn          func(ctx context.Context, req *lkproto.DeleteRoomRequest) (*lkproto.DeleteRoomResponse, error)
	listParticipantsFn    func(ctx context.Context, req *lkproto.ListParticipantsRequest) (*lkproto.ListParticipantsResponse, error)
	getParticipantFn      func(ctx context.Context, req *lkproto.RoomParticipantIdentity) (*lkproto.ParticipantInfo, error)
	removeParticipantFn   func(ctx context.Context, req *lkproto.RoomParticipantIdentity) (*lkproto.RemoveParticipantResponse, error)
	mutePublishedTrackFn  func(ctx context.Context, req *lkproto.MuteRoomTrackRequest) (*lkproto.MuteRoomTrackResponse, error)
	updateParticipantFn   func(ctx context.Context, req *lkproto.UpdateParticipantRequest) (*lkproto.ParticipantInfo, error)
	updateRoomMetadataFn  func(ctx context.Context, req *lkproto.UpdateRoomMetadataRequest) (*lkproto.Room, error)
	sendDataFn            func(ctx context.Context, req *lkproto.SendDataRequest) (*lkproto.SendDataResponse, error)
}

func (m *mockRoomClient) CreateRoom(ctx context.Context, req *lkproto.CreateRoomRequest) (*lkproto.Room, error) {
	if m.createRoomFn != nil {
		return m.createRoomFn(ctx, req)
	}
	return &lkproto.Room{Sid: "RM_test", Name: req.Name}, nil
}
func (m *mockRoomClient) ListRooms(ctx context.Context, req *lkproto.ListRoomsRequest) (*lkproto.ListRoomsResponse, error) {
	if m.listRoomsFn != nil {
		return m.listRoomsFn(ctx, req)
	}
	return &lkproto.ListRoomsResponse{Rooms: []*lkproto.Room{{Sid: "RM_1", Name: "room1"}}}, nil
}
func (m *mockRoomClient) DeleteRoom(ctx context.Context, req *lkproto.DeleteRoomRequest) (*lkproto.DeleteRoomResponse, error) {
	if m.deleteRoomFn != nil {
		return m.deleteRoomFn(ctx, req)
	}
	return &lkproto.DeleteRoomResponse{}, nil
}
func (m *mockRoomClient) ListParticipants(ctx context.Context, req *lkproto.ListParticipantsRequest) (*lkproto.ListParticipantsResponse, error) {
	if m.listParticipantsFn != nil {
		return m.listParticipantsFn(ctx, req)
	}
	return &lkproto.ListParticipantsResponse{Participants: []*lkproto.ParticipantInfo{{Sid: "PA_1", Identity: "user1"}}}, nil
}
func (m *mockRoomClient) GetParticipant(ctx context.Context, req *lkproto.RoomParticipantIdentity) (*lkproto.ParticipantInfo, error) {
	if m.getParticipantFn != nil {
		return m.getParticipantFn(ctx, req)
	}
	return &lkproto.ParticipantInfo{Sid: "PA_1", Identity: req.Identity, Name: "Test User"}, nil
}
func (m *mockRoomClient) RemoveParticipant(ctx context.Context, req *lkproto.RoomParticipantIdentity) (*lkproto.RemoveParticipantResponse, error) {
	if m.removeParticipantFn != nil {
		return m.removeParticipantFn(ctx, req)
	}
	return &lkproto.RemoveParticipantResponse{}, nil
}
func (m *mockRoomClient) MutePublishedTrack(ctx context.Context, req *lkproto.MuteRoomTrackRequest) (*lkproto.MuteRoomTrackResponse, error) {
	if m.mutePublishedTrackFn != nil {
		return m.mutePublishedTrackFn(ctx, req)
	}
	return &lkproto.MuteRoomTrackResponse{Track: &lkproto.TrackInfo{Sid: req.TrackSid, Name: "track", Muted: req.Muted}}, nil
}
func (m *mockRoomClient) UpdateParticipant(ctx context.Context, req *lkproto.UpdateParticipantRequest) (*lkproto.ParticipantInfo, error) {
	if m.updateParticipantFn != nil {
		return m.updateParticipantFn(ctx, req)
	}
	return &lkproto.ParticipantInfo{Sid: "PA_1", Identity: req.Identity, Metadata: req.Metadata}, nil
}
func (m *mockRoomClient) UpdateRoomMetadata(ctx context.Context, req *lkproto.UpdateRoomMetadataRequest) (*lkproto.Room, error) {
	if m.updateRoomMetadataFn != nil {
		return m.updateRoomMetadataFn(ctx, req)
	}
	return &lkproto.Room{Sid: "RM_1", Name: req.Room, Metadata: req.Metadata}, nil
}
func (m *mockRoomClient) SendData(ctx context.Context, req *lkproto.SendDataRequest) (*lkproto.SendDataResponse, error) {
	if m.sendDataFn != nil {
		return m.sendDataFn(ctx, req)
	}
	return &lkproto.SendDataResponse{}, nil
}

// --- Mock Egress Client ---

type mockEgressClient struct {
	startRoomCompositeEgressFn func(ctx context.Context, req *lkproto.RoomCompositeEgressRequest) (*lkproto.EgressInfo, error)
	startTrackEgressFn         func(ctx context.Context, req *lkproto.TrackEgressRequest) (*lkproto.EgressInfo, error)
	stopEgressFn               func(ctx context.Context, req *lkproto.StopEgressRequest) (*lkproto.EgressInfo, error)
	listEgressFn               func(ctx context.Context, req *lkproto.ListEgressRequest) (*lkproto.ListEgressResponse, error)
}

func (m *mockEgressClient) StartRoomCompositeEgress(ctx context.Context, req *lkproto.RoomCompositeEgressRequest) (*lkproto.EgressInfo, error) {
	if m.startRoomCompositeEgressFn != nil {
		return m.startRoomCompositeEgressFn(ctx, req)
	}
	return &lkproto.EgressInfo{EgressId: "EG_1", RoomName: req.RoomName, Status: lkproto.EgressStatus_EGRESS_STARTING}, nil
}
func (m *mockEgressClient) StartTrackEgress(ctx context.Context, req *lkproto.TrackEgressRequest) (*lkproto.EgressInfo, error) {
	if m.startTrackEgressFn != nil {
		return m.startTrackEgressFn(ctx, req)
	}
	return &lkproto.EgressInfo{EgressId: "EG_2", RoomName: req.RoomName, Status: lkproto.EgressStatus_EGRESS_STARTING}, nil
}
func (m *mockEgressClient) StopEgress(ctx context.Context, req *lkproto.StopEgressRequest) (*lkproto.EgressInfo, error) {
	if m.stopEgressFn != nil {
		return m.stopEgressFn(ctx, req)
	}
	return &lkproto.EgressInfo{EgressId: req.EgressId, Status: lkproto.EgressStatus_EGRESS_COMPLETE}, nil
}
func (m *mockEgressClient) ListEgress(ctx context.Context, req *lkproto.ListEgressRequest) (*lkproto.ListEgressResponse, error) {
	if m.listEgressFn != nil {
		return m.listEgressFn(ctx, req)
	}
	return &lkproto.ListEgressResponse{Items: []*lkproto.EgressInfo{{EgressId: "EG_1", RoomName: "room1"}}}, nil
}

// --- Mock Ingress Client ---

type mockIngressClient struct {
	createIngressFn func(ctx context.Context, req *lkproto.CreateIngressRequest) (*lkproto.IngressInfo, error)
	listIngressFn   func(ctx context.Context, req *lkproto.ListIngressRequest) (*lkproto.ListIngressResponse, error)
	deleteIngressFn func(ctx context.Context, req *lkproto.DeleteIngressRequest) (*lkproto.IngressInfo, error)
}

func (m *mockIngressClient) CreateIngress(ctx context.Context, req *lkproto.CreateIngressRequest) (*lkproto.IngressInfo, error) {
	if m.createIngressFn != nil {
		return m.createIngressFn(ctx, req)
	}
	return &lkproto.IngressInfo{
		IngressId:           "IN_1",
		Url:                 "rtmp://ingest.example.com/live",
		StreamKey:           "sk_test",
		RoomName:            req.RoomName,
		ParticipantIdentity: req.ParticipantIdentity,
		InputType:           req.InputType,
	}, nil
}
func (m *mockIngressClient) ListIngress(ctx context.Context, req *lkproto.ListIngressRequest) (*lkproto.ListIngressResponse, error) {
	if m.listIngressFn != nil {
		return m.listIngressFn(ctx, req)
	}
	return &lkproto.ListIngressResponse{Items: []*lkproto.IngressInfo{{IngressId: "IN_1", RoomName: "room1"}}}, nil
}
func (m *mockIngressClient) DeleteIngress(ctx context.Context, req *lkproto.DeleteIngressRequest) (*lkproto.IngressInfo, error) {
	if m.deleteIngressFn != nil {
		return m.deleteIngressFn(ctx, req)
	}
	return &lkproto.IngressInfo{IngressId: req.IngressId}, nil
}

// --- Test helpers ---

func testService() *Service {
	return &Service{
		Room:      &mockRoomClient{},
		Egress:    &mockEgressClient{},
		Ingress:   &mockIngressClient{},
		APIKey:    "test-key",
		APISecret: "test-secret-that-is-long-enough-for-jwt",
	}
}

func testServices(svc *Service) map[string]any {
	return map[string]any{serviceDep: svc}
}

// --- Token tests ---

func TestTokenNode_Success(t *testing.T) {
	svc := testService()
	exec := &tokenExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, data, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"identity": "user1", "room": "room1"},
		testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	result := data.(map[string]any)
	assert.NotEmpty(t, result["token"])
	assert.Equal(t, "user1", result["identity"])
	assert.Equal(t, "room1", result["room"])
}

func TestTokenNode_MissingIdentity(t *testing.T) {
	svc := testService()
	exec := &tokenExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"room": "room1"},
		testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

func TestTokenNode_MissingService(t *testing.T) {
	exec := &tokenExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"identity": "u", "room": "r"},
		map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service not configured")
}

func TestTokenNode_WithGrants(t *testing.T) {
	svc := testService()
	exec := &tokenExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, data, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"identity": "user1",
			"room":     "room1",
			"grants": map[string]any{
				"canPublish":   true,
				"canSubscribe": true,
				"roomAdmin":    false,
			},
		},
		testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.NotEmpty(t, data.(map[string]any)["token"])
}

func TestTokenNode_InvalidTTL(t *testing.T) {
	svc := testService()
	exec := &tokenExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"identity": "u", "room": "r", "ttl": "invalid"},
		testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid ttl")
}

// --- Room Create tests ---

func TestRoomCreateNode_Success(t *testing.T) {
	svc := testService()
	exec := &roomCreateExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, data, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"name": "my-room"},
		testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, "my-room", data.(map[string]any)["name"])
}

func TestRoomCreateNode_MissingName(t *testing.T) {
	svc := testService()
	exec := &roomCreateExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{}, testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

func TestRoomCreateNode_SDKError(t *testing.T) {
	svc := testService()
	svc.Room = &mockRoomClient{
		createRoomFn: func(_ context.Context, _ *lkproto.CreateRoomRequest) (*lkproto.Room, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}
	exec := &roomCreateExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"name": "room"},
		testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
}

// --- Room List tests ---

func TestRoomListNode_Success(t *testing.T) {
	svc := testService()
	exec := &roomListExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, data, err := exec.Execute(context.Background(), nCtx,
		map[string]any{}, testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	rooms := data.(map[string]any)["rooms"].([]any)
	assert.Len(t, rooms, 1)
}

// --- Room Delete tests ---

func TestRoomDeleteNode_Success(t *testing.T) {
	svc := testService()
	exec := &roomDeleteExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, data, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"room": "my-room"},
		testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, true, data.(map[string]any)["deleted"])
}

// --- Room Update Metadata tests ---

func TestRoomUpdateMetadataNode_Success(t *testing.T) {
	svc := testService()
	exec := &roomUpdateMetadataExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, data, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"room": "my-room", "metadata": `{"key":"val"}`},
		testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, `{"key":"val"}`, data.(map[string]any)["metadata"])
}

// --- SendData tests ---

func TestSendDataNode_Success(t *testing.T) {
	svc := testService()
	exec := &sendDataExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, data, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"room": "my-room", "data": "hello"},
		testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, true, data.(map[string]any)["sent"])
}

func TestSendDataNode_ObjectData(t *testing.T) {
	svc := testService()
	exec := &sendDataExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"room": "r", "data": map[string]any{"msg": "hi"}},
		testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
}

// --- Participant List tests ---

func TestParticipantListNode_Success(t *testing.T) {
	svc := testService()
	exec := &participantListExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, data, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"room": "my-room"},
		testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	participants := data.(map[string]any)["participants"].([]any)
	assert.Len(t, participants, 1)
}

// --- Participant Get tests ---

func TestParticipantGetNode_Success(t *testing.T) {
	svc := testService()
	exec := &participantGetExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, data, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"room": "my-room", "identity": "user1"},
		testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, "user1", data.(map[string]any)["identity"])
}

func TestParticipantGetNode_SDKError(t *testing.T) {
	svc := testService()
	svc.Room = &mockRoomClient{
		getParticipantFn: func(_ context.Context, _ *lkproto.RoomParticipantIdentity) (*lkproto.ParticipantInfo, error) {
			return nil, fmt.Errorf("participant not found")
		},
	}
	exec := &participantGetExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"room": "r", "identity": "missing"},
		testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "participant not found")
}

// --- Participant Remove tests ---

func TestParticipantRemoveNode_Success(t *testing.T) {
	svc := testService()
	exec := &participantRemoveExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, data, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"room": "my-room", "identity": "user1"},
		testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, true, data.(map[string]any)["removed"])
}

// --- Participant Update tests ---

func TestParticipantUpdateNode_Success(t *testing.T) {
	svc := testService()
	exec := &participantUpdateExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, data, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"room":     "my-room",
			"identity": "user1",
			"metadata": `{"role":"speaker"}`,
		},
		testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, `{"role":"speaker"}`, data.(map[string]any)["metadata"])
}

func TestParticipantUpdateNode_WithPermissions(t *testing.T) {
	svc := testService()
	exec := &participantUpdateExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"room":     "r",
			"identity": "u",
			"permissions": map[string]any{
				"canPublish":   true,
				"canSubscribe": false,
			},
		},
		testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
}

// --- Mute Track tests ---

func TestMuteTrackNode_Success(t *testing.T) {
	svc := testService()
	exec := &muteTrackExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, data, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"room":      "my-room",
			"identity":  "user1",
			"track_sid": "TR_abc",
			"muted":     true,
		},
		testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, true, data.(map[string]any)["muted"])
	assert.Equal(t, "TR_abc", data.(map[string]any)["track_sid"])
}

func TestMuteTrackNode_MissingTrackSID(t *testing.T) {
	svc := testService()
	exec := &muteTrackExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"room": "r", "identity": "u", "muted": true},
		testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

// --- Egress Start Room Composite tests ---

func TestEgressStartRoomCompositeNode_Success(t *testing.T) {
	svc := testService()
	exec := &egressStartRoomCompositeExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, data, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"room": "my-room",
			"output": map[string]any{
				"type":     "s3",
				"bucket":   "recordings",
				"filepath": "rec.mp4",
			},
		},
		testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, "EG_1", data.(map[string]any)["egress_id"])
}

func TestEgressStartRoomCompositeNode_MissingRoom(t *testing.T) {
	svc := testService()
	exec := &egressStartRoomCompositeExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"output": map[string]any{"type": "file"}},
		testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

// --- Egress Start Track tests ---

func TestEgressStartTrackNode_Success(t *testing.T) {
	svc := testService()
	exec := &egressStartTrackExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, data, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"room":      "my-room",
			"track_sid": "TR_abc",
			"output": map[string]any{
				"type":     "file",
				"filepath": "/tmp/track.ogg",
			},
		},
		testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, "EG_2", data.(map[string]any)["egress_id"])
}

// --- Egress Stop tests ---

func TestEgressStopNode_Success(t *testing.T) {
	svc := testService()
	exec := &egressStopExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, data, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"egress_id": "EG_1"},
		testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, "EG_1", data.(map[string]any)["egress_id"])
}

// --- Egress List tests ---

func TestEgressListNode_Success(t *testing.T) {
	svc := testService()
	exec := &egressListExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, data, err := exec.Execute(context.Background(), nCtx,
		map[string]any{}, testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	items := data.(map[string]any)["items"].([]any)
	assert.Len(t, items, 1)
}

// --- Ingress Create tests ---

func TestIngressCreateNode_Success(t *testing.T) {
	svc := testService()
	exec := &ingressCreateExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, data, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"input_type":           "rtmp",
			"room":                 "my-room",
			"participant_identity": "streamer1",
		},
		testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	result := data.(map[string]any)
	assert.Equal(t, "IN_1", result["ingress_id"])
	assert.NotEmpty(t, result["url"])
	assert.NotEmpty(t, result["stream_key"])
}

func TestIngressCreateNode_InvalidInputType(t *testing.T) {
	svc := testService()
	exec := &ingressCreateExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"input_type":           "invalid",
			"room":                 "r",
			"participant_identity": "p",
		},
		testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported input_type")
}

// --- Ingress List tests ---

func TestIngressListNode_Success(t *testing.T) {
	svc := testService()
	exec := &ingressListExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, data, err := exec.Execute(context.Background(), nCtx,
		map[string]any{}, testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	items := data.(map[string]any)["items"].([]any)
	assert.Len(t, items, 1)
}

// --- Ingress Delete tests ---

func TestIngressDeleteNode_Success(t *testing.T) {
	svc := testService()
	exec := &ingressDeleteExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, data, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"ingress_id": "IN_1"},
		testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, true, data.(map[string]any)["deleted"])
}
