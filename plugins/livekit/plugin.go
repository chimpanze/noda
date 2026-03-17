package livekit

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
	lkproto "github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
)

// Plugin implements the LiveKit plugin for Noda.
type Plugin struct{}

func (p *Plugin) Name() string   { return "livekit" }
func (p *Plugin) Prefix() string { return "lk" }

func (p *Plugin) HasServices() bool { return true }

func (p *Plugin) Nodes() []api.NodeRegistration {
	return []api.NodeRegistration{
		// Token
		{Descriptor: &tokenDescriptor{}, Factory: newTokenExecutor},
		// Room
		{Descriptor: &roomCreateDescriptor{}, Factory: newRoomCreateExecutor},
		{Descriptor: &roomListDescriptor{}, Factory: newRoomListExecutor},
		{Descriptor: &roomDeleteDescriptor{}, Factory: newRoomDeleteExecutor},
		{Descriptor: &roomUpdateMetadataDescriptor{}, Factory: newRoomUpdateMetadataExecutor},
		{Descriptor: &sendDataDescriptor{}, Factory: newSendDataExecutor},
		// Participant
		{Descriptor: &participantListDescriptor{}, Factory: newParticipantListExecutor},
		{Descriptor: &participantGetDescriptor{}, Factory: newParticipantGetExecutor},
		{Descriptor: &participantRemoveDescriptor{}, Factory: newParticipantRemoveExecutor},
		{Descriptor: &participantUpdateDescriptor{}, Factory: newParticipantUpdateExecutor},
		{Descriptor: &muteTrackDescriptor{}, Factory: newMuteTrackExecutor},
		// Egress
		{Descriptor: &egressStartRoomCompositeDescriptor{}, Factory: newEgressStartRoomCompositeExecutor},
		{Descriptor: &egressStartTrackDescriptor{}, Factory: newEgressStartTrackExecutor},
		{Descriptor: &egressStopDescriptor{}, Factory: newEgressStopExecutor},
		{Descriptor: &egressListDescriptor{}, Factory: newEgressListExecutor},
		// Ingress
		{Descriptor: &ingressCreateDescriptor{}, Factory: newIngressCreateExecutor},
		{Descriptor: &ingressListDescriptor{}, Factory: newIngressListExecutor},
		{Descriptor: &ingressDeleteDescriptor{}, Factory: newIngressDeleteExecutor},
	}
}

func (p *Plugin) CreateService(config map[string]any) (any, error) {
	url, _ := config["url"].(string)
	if url == "" {
		return nil, fmt.Errorf("livekit: missing required field \"url\"")
	}
	apiKey, _ := config["api_key"].(string)
	if apiKey == "" {
		return nil, fmt.Errorf("livekit: missing required field \"api_key\"")
	}
	apiSecret, _ := config["api_secret"].(string)
	if apiSecret == "" {
		return nil, fmt.Errorf("livekit: missing required field \"api_secret\"")
	}

	roomClient := lksdk.NewRoomServiceClient(url, apiKey, apiSecret)
	egressClient := lksdk.NewEgressClient(url, apiKey, apiSecret)
	ingressClient := lksdk.NewIngressClient(url, apiKey, apiSecret)

	return &Service{
		Room:      roomClient,
		Egress:    egressClient,
		Ingress:   ingressClient,
		APIKey:    apiKey,
		APISecret: apiSecret,
	}, nil
}

func (p *Plugin) HealthCheck(service any) error {
	svc, ok := service.(*Service)
	if !ok {
		return fmt.Errorf("livekit: invalid service type")
	}
	_, err := svc.Room.ListRooms(context.Background(), &lkproto.ListRoomsRequest{})
	if err != nil {
		return fmt.Errorf("livekit: health check failed: %w", err)
	}
	return nil
}

func (p *Plugin) Shutdown(_ any) error {
	return nil // SDK clients are stateless
}
