package livekit

import (
	"context"

	lkproto "github.com/livekit/protocol/livekit"
)

// RoomClient wraps LiveKit's RoomServiceClient for testability.
type RoomClient interface {
	CreateRoom(ctx context.Context, req *lkproto.CreateRoomRequest) (*lkproto.Room, error)
	ListRooms(ctx context.Context, req *lkproto.ListRoomsRequest) (*lkproto.ListRoomsResponse, error)
	DeleteRoom(ctx context.Context, req *lkproto.DeleteRoomRequest) (*lkproto.DeleteRoomResponse, error)
	ListParticipants(ctx context.Context, req *lkproto.ListParticipantsRequest) (*lkproto.ListParticipantsResponse, error)
	GetParticipant(ctx context.Context, req *lkproto.RoomParticipantIdentity) (*lkproto.ParticipantInfo, error)
	RemoveParticipant(ctx context.Context, req *lkproto.RoomParticipantIdentity) (*lkproto.RemoveParticipantResponse, error)
	MutePublishedTrack(ctx context.Context, req *lkproto.MuteRoomTrackRequest) (*lkproto.MuteRoomTrackResponse, error)
	UpdateParticipant(ctx context.Context, req *lkproto.UpdateParticipantRequest) (*lkproto.ParticipantInfo, error)
	UpdateRoomMetadata(ctx context.Context, req *lkproto.UpdateRoomMetadataRequest) (*lkproto.Room, error)
	SendData(ctx context.Context, req *lkproto.SendDataRequest) (*lkproto.SendDataResponse, error)
}

// EgressClient wraps LiveKit's EgressClient for testability.
type EgressClient interface {
	StartRoomCompositeEgress(ctx context.Context, req *lkproto.RoomCompositeEgressRequest) (*lkproto.EgressInfo, error)
	StartTrackEgress(ctx context.Context, req *lkproto.TrackEgressRequest) (*lkproto.EgressInfo, error)
	StopEgress(ctx context.Context, req *lkproto.StopEgressRequest) (*lkproto.EgressInfo, error)
	ListEgress(ctx context.Context, req *lkproto.ListEgressRequest) (*lkproto.ListEgressResponse, error)
}

// IngressClient wraps LiveKit's IngressClient for testability.
type IngressClient interface {
	CreateIngress(ctx context.Context, req *lkproto.CreateIngressRequest) (*lkproto.IngressInfo, error)
	ListIngress(ctx context.Context, req *lkproto.ListIngressRequest) (*lkproto.ListIngressResponse, error)
	DeleteIngress(ctx context.Context, req *lkproto.DeleteIngressRequest) (*lkproto.IngressInfo, error)
}
