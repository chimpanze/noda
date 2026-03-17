package livekit

import (
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	lkproto "github.com/livekit/protocol/livekit"
)

// buildFileOutput resolves the "output" config map into an EncodedFileOutput.
func buildFileOutput(nCtx api.ExecutionContext, config map[string]any) (*lkproto.EncodedFileOutput, error) {
	outputMap, err := plugin.ResolveMap(nCtx, config, "output")
	if err != nil {
		return nil, err
	}

	out := &lkproto.EncodedFileOutput{}

	if fp, ok := outputMap["filepath"].(string); ok {
		out.Filepath = fp
	}

	storageType, _ := outputMap["type"].(string)
	switch storageType {
	case "s3":
		s3 := &lkproto.S3Upload{}
		if v, ok := outputMap["bucket"].(string); ok {
			s3.Bucket = v
		}
		if v, ok := outputMap["region"].(string); ok {
			s3.Region = v
		}
		if v, ok := outputMap["access_key"].(string); ok {
			s3.AccessKey = v
		}
		if v, ok := outputMap["secret"].(string); ok {
			s3.Secret = v
		}
		if v, ok := outputMap["endpoint"].(string); ok {
			s3.Endpoint = v
		}
		out.Output = &lkproto.EncodedFileOutput_S3{S3: s3}
	case "gcs":
		gcs := &lkproto.GCPUpload{}
		if v, ok := outputMap["bucket"].(string); ok {
			gcs.Bucket = v
		}
		if v, ok := outputMap["credentials"].(string); ok {
			gcs.Credentials = v
		}
		out.Output = &lkproto.EncodedFileOutput_Gcp{Gcp: gcs}
	case "azure":
		az := &lkproto.AzureBlobUpload{}
		if v, ok := outputMap["account_name"].(string); ok {
			az.AccountName = v
		}
		if v, ok := outputMap["account_key"].(string); ok {
			az.AccountKey = v
		}
		if v, ok := outputMap["container_name"].(string); ok {
			az.ContainerName = v
		}
		out.Output = &lkproto.EncodedFileOutput_Azure{Azure: az}
	case "file", "":
		// Local file output — no upload config needed
	default:
		return nil, fmt.Errorf("unsupported output type: %q", storageType)
	}

	return out, nil
}

func egressInfoToMap(info *lkproto.EgressInfo) map[string]any {
	return map[string]any{
		"egress_id":  info.EgressId,
		"room_id":    info.RoomId,
		"room_name":  info.RoomName,
		"status":     info.Status.String(),
		"started_at": info.StartedAt,
		"ended_at":   info.EndedAt,
	}
}
