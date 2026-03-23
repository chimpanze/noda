package livekit

import (
	"testing"

	"github.com/livekit/protocol/auth"
	lkproto "github.com/livekit/protocol/livekit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- applyGrants tests ---

func TestApplyGrants_AllBoolFields(t *testing.T) {
	vg := &auth.VideoGrant{}
	grants := map[string]any{
		"roomJoin":   true,
		"roomCreate": true,
		"roomList":   true,
		"roomAdmin":  true,
		"hidden":     true,
		"recorder":   true,
	}
	applyGrants(grants, vg)

	assert.True(t, vg.RoomJoin)
	assert.True(t, vg.RoomCreate)
	assert.True(t, vg.RoomList)
	assert.True(t, vg.RoomAdmin)
	assert.True(t, vg.Hidden)
	assert.True(t, vg.Recorder)
}

func TestApplyGrants_SetterFields(t *testing.T) {
	vg := &auth.VideoGrant{}
	grants := map[string]any{
		"canPublish":           true,
		"canSubscribe":         true,
		"canPublishData":       true,
		"canUpdateOwnMetadata": true,
	}
	applyGrants(grants, vg)

	assert.True(t, vg.GetCanPublish())
	assert.True(t, vg.GetCanSubscribe())
	assert.True(t, vg.GetCanPublishData())
	assert.True(t, vg.GetCanUpdateOwnMetadata())
}

func TestApplyGrants_CanPublishSources(t *testing.T) {
	vg := &auth.VideoGrant{}
	grants := map[string]any{
		"canPublishSources": []any{"CAMERA", "MICROPHONE"},
	}
	applyGrants(grants, vg)

	sources := vg.GetCanPublishSources()
	assert.Len(t, sources, 2)
}

func TestApplyGrants_CanPublishSources_InvalidSource(t *testing.T) {
	vg := &auth.VideoGrant{}
	grants := map[string]any{
		"canPublishSources": []any{"CAMERA", "INVALID_SOURCE", 42},
	}
	applyGrants(grants, vg)

	sources := vg.GetCanPublishSources()
	// Only CAMERA should be included; INVALID_SOURCE is not in TrackSource_value, 42 is not a string
	assert.Len(t, sources, 1)
}

func TestApplyGrants_EmptyMap(t *testing.T) {
	vg := &auth.VideoGrant{}
	applyGrants(map[string]any{}, vg)

	// Nothing should be set
	assert.False(t, vg.RoomJoin)
	assert.False(t, vg.RoomCreate)
}

func TestApplyGrants_WrongTypes(t *testing.T) {
	vg := &auth.VideoGrant{}
	grants := map[string]any{
		"roomJoin":          "not a bool",
		"canPublishSources": "not an array",
	}
	applyGrants(grants, vg)

	// roomJoin should not be set since type doesn't match
	assert.False(t, vg.RoomJoin)
	// canPublishSources should not be set since type doesn't match
	assert.Empty(t, vg.GetCanPublishSources())
}

// --- roomToMap tests ---

func TestRoomToMap(t *testing.T) {
	room := &lkproto.Room{
		Sid:             "RM_abc",
		Name:            "test-room",
		EmptyTimeout:    300,
		MaxParticipants: 50,
		Metadata:        `{"key":"val"}`,
		NumParticipants: 5,
		CreationTime:    1234567890,
		ActiveRecording: true,
	}
	m := roomToMap(room)

	assert.Equal(t, "RM_abc", m["sid"])
	assert.Equal(t, "test-room", m["name"])
	assert.Equal(t, uint32(300), m["empty_timeout"])
	assert.Equal(t, uint32(50), m["max_participants"])
	assert.Equal(t, `{"key":"val"}`, m["metadata"])
	assert.Equal(t, uint32(5), m["num_participants"])
	assert.Equal(t, int64(1234567890), m["creation_time"])
	assert.Equal(t, true, m["active_recording"])
}

// --- participantToMap tests ---

func TestParticipantToMap(t *testing.T) {
	p := &lkproto.ParticipantInfo{
		Sid:      "PA_abc",
		Identity: "user1",
		Name:     "User One",
		Metadata: `{"role":"admin"}`,
		State:    lkproto.ParticipantInfo_ACTIVE,
		JoinedAt: 1234567890,
		Region:   "us-west-2",
	}
	m := participantToMap(p)

	assert.Equal(t, "PA_abc", m["sid"])
	assert.Equal(t, "user1", m["identity"])
	assert.Equal(t, "User One", m["name"])
	assert.Equal(t, `{"role":"admin"}`, m["metadata"])
	assert.Equal(t, "ACTIVE", m["state"])
	assert.Equal(t, int64(1234567890), m["joined_at"])
	assert.Equal(t, "us-west-2", m["region"])
}

func TestParticipantToMap_DefaultState(t *testing.T) {
	p := &lkproto.ParticipantInfo{
		Sid:      "PA_1",
		Identity: "u",
	}
	m := participantToMap(p)
	assert.Equal(t, "JOINING", m["state"])
}

// --- egressInfoToMap tests ---

func TestEgressInfoToMap(t *testing.T) {
	info := &lkproto.EgressInfo{
		EgressId:  "EG_abc",
		RoomId:    "RM_123",
		RoomName:  "my-room",
		Status:    lkproto.EgressStatus_EGRESS_ACTIVE,
		StartedAt: 1000,
		EndedAt:   2000,
	}
	m := egressInfoToMap(info)

	assert.Equal(t, "EG_abc", m["egress_id"])
	assert.Equal(t, "RM_123", m["room_id"])
	assert.Equal(t, "my-room", m["room_name"])
	assert.Equal(t, "EGRESS_ACTIVE", m["status"])
	assert.Equal(t, int64(1000), m["started_at"])
	assert.Equal(t, int64(2000), m["ended_at"])
}

// --- ingressInfoToMap tests ---

func TestIngressInfoToMap(t *testing.T) {
	info := &lkproto.IngressInfo{
		IngressId:           "IN_abc",
		Url:                 "rtmp://ingest.example.com/live",
		StreamKey:           "sk_123",
		RoomName:            "my-room",
		ParticipantIdentity: "streamer",
		ParticipantName:     "Streamer Name",
		InputType:           lkproto.IngressInput_RTMP_INPUT,
	}
	m := ingressInfoToMap(info)

	assert.Equal(t, "IN_abc", m["ingress_id"])
	assert.Equal(t, "rtmp://ingest.example.com/live", m["url"])
	assert.Equal(t, "sk_123", m["stream_key"])
	assert.Equal(t, "my-room", m["room"])
	assert.Equal(t, "streamer", m["participant_identity"])
	assert.Equal(t, "Streamer Name", m["participant_name"])
	assert.Equal(t, "RTMP_INPUT", m["input_type"])
}

// --- buildFileOutput tests ---

func TestBuildFileOutput_S3(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"output": map[string]any{
			"type":       "s3",
			"filepath":   "recordings/test.mp4",
			"bucket":     "my-bucket",
			"region":     "us-east-1",
			"access_key": "AKID",
			"secret":     "SECRET",
			"endpoint":   "https://s3.example.com",
		},
	}
	out, err := buildFileOutput(nCtx, config)
	require.NoError(t, err)
	assert.Equal(t, "recordings/test.mp4", out.Filepath)

	s3 := out.Output.(*lkproto.EncodedFileOutput_S3).S3
	assert.Equal(t, "my-bucket", s3.Bucket)
	assert.Equal(t, "us-east-1", s3.Region)
	assert.Equal(t, "AKID", s3.AccessKey)
	assert.Equal(t, "SECRET", s3.Secret)
	assert.Equal(t, "https://s3.example.com", s3.Endpoint)
}

func TestBuildFileOutput_GCS(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"output": map[string]any{
			"type":        "gcs",
			"filepath":    "recordings/test.mp4",
			"bucket":      "gcs-bucket",
			"credentials": "cred-json",
		},
	}
	out, err := buildFileOutput(nCtx, config)
	require.NoError(t, err)
	assert.Equal(t, "recordings/test.mp4", out.Filepath)

	gcs := out.Output.(*lkproto.EncodedFileOutput_Gcp).Gcp
	assert.Equal(t, "gcs-bucket", gcs.Bucket)
	assert.Equal(t, "cred-json", gcs.Credentials)
}

func TestBuildFileOutput_Azure(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"output": map[string]any{
			"type":           "azure",
			"filepath":       "recordings/test.mp4",
			"account_name":   "myaccount",
			"account_key":    "mykey",
			"container_name": "mycontainer",
		},
	}
	out, err := buildFileOutput(nCtx, config)
	require.NoError(t, err)

	az := out.Output.(*lkproto.EncodedFileOutput_Azure).Azure
	assert.Equal(t, "myaccount", az.AccountName)
	assert.Equal(t, "mykey", az.AccountKey)
	assert.Equal(t, "mycontainer", az.ContainerName)
}

func TestBuildFileOutput_LocalFile(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"output": map[string]any{
			"type":     "file",
			"filepath": "/tmp/recording.mp4",
		},
	}
	out, err := buildFileOutput(nCtx, config)
	require.NoError(t, err)
	assert.Equal(t, "/tmp/recording.mp4", out.Filepath)
	assert.Nil(t, out.Output)
}

func TestBuildFileOutput_EmptyType(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"output": map[string]any{
			"filepath": "/tmp/recording.mp4",
		},
	}
	out, err := buildFileOutput(nCtx, config)
	require.NoError(t, err)
	assert.Equal(t, "/tmp/recording.mp4", out.Filepath)
	assert.Nil(t, out.Output)
}

func TestBuildFileOutput_UnsupportedType(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"output": map[string]any{
			"type": "ftp",
		},
	}
	_, err := buildFileOutput(nCtx, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported output type")
}

func TestBuildFileOutput_MissingOutput(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{}
	_, err := buildFileOutput(nCtx, config)
	require.Error(t, err)
}

// --- Service.NewAuthProvider tests ---

func TestService_NewAuthProvider(t *testing.T) {
	svc := &Service{
		APIKey:    "my-api-key",
		APISecret: "my-api-secret",
	}
	provider := svc.newAuthProvider()
	require.NotNil(t, provider)

	secret := provider.GetSecret("my-api-key")
	assert.Equal(t, "my-api-secret", secret)
}
