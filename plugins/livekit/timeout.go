package livekit

import (
	"context"
	"errors"
	"fmt"
	"time"

	lkproto "github.com/livekit/protocol/livekit"
)

// callWithTimeout bounds one LiveKit API call by the service-level timeout.
// A deadline expiry produces an operation-scoped error; errors returned by
// the server before the deadline pass through unchanged.
func callWithTimeout[T any](ctx context.Context, d time.Duration, op string, call func(context.Context) (T, error)) (T, error) {
	tctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	res, err := call(tctx)
	if err != nil && errors.Is(tctx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
		var zero T
		return zero, fmt.Errorf("livekit %s: request timed out after %s: %w", op, d, err)
	}
	return res, err
}

// timeoutRoomClient wraps a RoomClient with a per-call deadline.
type timeoutRoomClient struct {
	inner RoomClient
	d     time.Duration
}

func (c *timeoutRoomClient) CreateRoom(ctx context.Context, req *lkproto.CreateRoomRequest) (*lkproto.Room, error) {
	return callWithTimeout(ctx, c.d, "CreateRoom", func(ctx context.Context) (*lkproto.Room, error) { return c.inner.CreateRoom(ctx, req) })
}

func (c *timeoutRoomClient) ListRooms(ctx context.Context, req *lkproto.ListRoomsRequest) (*lkproto.ListRoomsResponse, error) {
	return callWithTimeout(ctx, c.d, "ListRooms", func(ctx context.Context) (*lkproto.ListRoomsResponse, error) { return c.inner.ListRooms(ctx, req) })
}

func (c *timeoutRoomClient) DeleteRoom(ctx context.Context, req *lkproto.DeleteRoomRequest) (*lkproto.DeleteRoomResponse, error) {
	return callWithTimeout(ctx, c.d, "DeleteRoom", func(ctx context.Context) (*lkproto.DeleteRoomResponse, error) { return c.inner.DeleteRoom(ctx, req) })
}

func (c *timeoutRoomClient) ListParticipants(ctx context.Context, req *lkproto.ListParticipantsRequest) (*lkproto.ListParticipantsResponse, error) {
	return callWithTimeout(ctx, c.d, "ListParticipants", func(ctx context.Context) (*lkproto.ListParticipantsResponse, error) {
		return c.inner.ListParticipants(ctx, req)
	})
}

func (c *timeoutRoomClient) GetParticipant(ctx context.Context, req *lkproto.RoomParticipantIdentity) (*lkproto.ParticipantInfo, error) {
	return callWithTimeout(ctx, c.d, "GetParticipant", func(ctx context.Context) (*lkproto.ParticipantInfo, error) { return c.inner.GetParticipant(ctx, req) })
}

func (c *timeoutRoomClient) RemoveParticipant(ctx context.Context, req *lkproto.RoomParticipantIdentity) (*lkproto.RemoveParticipantResponse, error) {
	return callWithTimeout(ctx, c.d, "RemoveParticipant", func(ctx context.Context) (*lkproto.RemoveParticipantResponse, error) {
		return c.inner.RemoveParticipant(ctx, req)
	})
}

func (c *timeoutRoomClient) MutePublishedTrack(ctx context.Context, req *lkproto.MuteRoomTrackRequest) (*lkproto.MuteRoomTrackResponse, error) {
	return callWithTimeout(ctx, c.d, "MutePublishedTrack", func(ctx context.Context) (*lkproto.MuteRoomTrackResponse, error) {
		return c.inner.MutePublishedTrack(ctx, req)
	})
}

func (c *timeoutRoomClient) UpdateParticipant(ctx context.Context, req *lkproto.UpdateParticipantRequest) (*lkproto.ParticipantInfo, error) {
	return callWithTimeout(ctx, c.d, "UpdateParticipant", func(ctx context.Context) (*lkproto.ParticipantInfo, error) {
		return c.inner.UpdateParticipant(ctx, req)
	})
}

func (c *timeoutRoomClient) UpdateRoomMetadata(ctx context.Context, req *lkproto.UpdateRoomMetadataRequest) (*lkproto.Room, error) {
	return callWithTimeout(ctx, c.d, "UpdateRoomMetadata", func(ctx context.Context) (*lkproto.Room, error) {
		return c.inner.UpdateRoomMetadata(ctx, req)
	})
}

func (c *timeoutRoomClient) SendData(ctx context.Context, req *lkproto.SendDataRequest) (*lkproto.SendDataResponse, error) {
	return callWithTimeout(ctx, c.d, "SendData", func(ctx context.Context) (*lkproto.SendDataResponse, error) { return c.inner.SendData(ctx, req) })
}

// timeoutEgressClient wraps an EgressClient with a per-call deadline.
type timeoutEgressClient struct {
	inner EgressClient
	d     time.Duration
}

func (c *timeoutEgressClient) StartRoomCompositeEgress(ctx context.Context, req *lkproto.RoomCompositeEgressRequest) (*lkproto.EgressInfo, error) {
	return callWithTimeout(ctx, c.d, "StartRoomCompositeEgress", func(ctx context.Context) (*lkproto.EgressInfo, error) {
		return c.inner.StartRoomCompositeEgress(ctx, req)
	})
}

func (c *timeoutEgressClient) StartTrackEgress(ctx context.Context, req *lkproto.TrackEgressRequest) (*lkproto.EgressInfo, error) {
	return callWithTimeout(ctx, c.d, "StartTrackEgress", func(ctx context.Context) (*lkproto.EgressInfo, error) {
		return c.inner.StartTrackEgress(ctx, req)
	})
}

func (c *timeoutEgressClient) StopEgress(ctx context.Context, req *lkproto.StopEgressRequest) (*lkproto.EgressInfo, error) {
	return callWithTimeout(ctx, c.d, "StopEgress", func(ctx context.Context) (*lkproto.EgressInfo, error) { return c.inner.StopEgress(ctx, req) })
}

func (c *timeoutEgressClient) ListEgress(ctx context.Context, req *lkproto.ListEgressRequest) (*lkproto.ListEgressResponse, error) {
	return callWithTimeout(ctx, c.d, "ListEgress", func(ctx context.Context) (*lkproto.ListEgressResponse, error) { return c.inner.ListEgress(ctx, req) })
}

// timeoutIngressClient wraps an IngressClient with a per-call deadline.
type timeoutIngressClient struct {
	inner IngressClient
	d     time.Duration
}

func (c *timeoutIngressClient) CreateIngress(ctx context.Context, req *lkproto.CreateIngressRequest) (*lkproto.IngressInfo, error) {
	return callWithTimeout(ctx, c.d, "CreateIngress", func(ctx context.Context) (*lkproto.IngressInfo, error) { return c.inner.CreateIngress(ctx, req) })
}

func (c *timeoutIngressClient) ListIngress(ctx context.Context, req *lkproto.ListIngressRequest) (*lkproto.ListIngressResponse, error) {
	return callWithTimeout(ctx, c.d, "ListIngress", func(ctx context.Context) (*lkproto.ListIngressResponse, error) { return c.inner.ListIngress(ctx, req) })
}

func (c *timeoutIngressClient) DeleteIngress(ctx context.Context, req *lkproto.DeleteIngressRequest) (*lkproto.IngressInfo, error) {
	return callWithTimeout(ctx, c.d, "DeleteIngress", func(ctx context.Context) (*lkproto.IngressInfo, error) { return c.inner.DeleteIngress(ctx, req) })
}
