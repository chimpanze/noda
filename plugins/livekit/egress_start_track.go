package livekit

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	lkproto "github.com/livekit/protocol/livekit"
)

type egressStartTrackDescriptor struct{}

func (d *egressStartTrackDescriptor) Name() string        { return "egressStartTrack" }
func (d *egressStartTrackDescriptor) Description() string { return "Starts a single track egress" }
func (d *egressStartTrackDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{serviceDep: {Prefix: "lk", Required: true}}
}
func (d *egressStartTrackDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"room":      map[string]any{"type": "string", "description": "Room name"},
			"track_sid": map[string]any{"type": "string", "description": "Track SID to record"},
			"output":    map[string]any{"type": "object", "description": "Output config (type, bucket, filepath, etc.)"},
		},
		"required": []any{"room", "track_sid", "output"},
	}
}
func (d *egressStartTrackDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "Egress info with egress_id",
		"error":   "Failed to start track egress",
	}
}

type egressStartTrackExecutor struct{}

func newEgressStartTrackExecutor(_ map[string]any) api.NodeExecutor {
	return &egressStartTrackExecutor{}
}

func (e *egressStartTrackExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *egressStartTrackExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[*Service](services, serviceDep)
	if err != nil {
		return "", nil, fmt.Errorf("lk.egressStartTrack: %w", err)
	}

	room, err := plugin.ResolveString(nCtx, config, "room")
	if err != nil {
		return "", nil, fmt.Errorf("lk.egressStartTrack: %w", err)
	}

	trackSID, err := plugin.ResolveString(nCtx, config, "track_sid")
	if err != nil {
		return "", nil, fmt.Errorf("lk.egressStartTrack: %w", err)
	}

	fileOutput, err := buildFileOutput(nCtx, config)
	if err != nil {
		return "", nil, fmt.Errorf("lk.egressStartTrack: %w", err)
	}

	req := &lkproto.TrackEgressRequest{
		RoomName: room,
		TrackId:  trackSID,
		Output: &lkproto.TrackEgressRequest_File{
			File: &lkproto.DirectFileOutput{
				Filepath: fileOutput.Filepath,
				Output:   nil, // inherit from fileOutput
			},
		},
	}

	// Map file output's upload config to direct file output
	if fileOutput.Output != nil {
		fileOut, ok := req.Output.(*lkproto.TrackEgressRequest_File)
		if !ok || fileOut == nil {
			return "", nil, fmt.Errorf("lk.egressStartTrack: unexpected output type")
		}
		directOut := fileOut.File
		switch v := fileOutput.Output.(type) {
		case *lkproto.EncodedFileOutput_S3:
			directOut.Output = &lkproto.DirectFileOutput_S3{S3: v.S3}
		case *lkproto.EncodedFileOutput_Gcp:
			directOut.Output = &lkproto.DirectFileOutput_Gcp{Gcp: v.Gcp}
		case *lkproto.EncodedFileOutput_Azure:
			directOut.Output = &lkproto.DirectFileOutput_Azure{Azure: v.Azure}
		}
	}

	info, err := svc.Egress.StartTrackEgress(ctx, req)
	if err != nil {
		return "", nil, fmt.Errorf("lk.egressStartTrack: %w", err)
	}

	return api.OutputSuccess, egressInfoToMap(info), nil
}
