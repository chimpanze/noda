package image

import "github.com/chimpanze/noda/pkg/api"

// Plugin registers all image.* nodes for bimg-based image processing.
type Plugin struct{}

func (p *Plugin) Name() string   { return "image" }
func (p *Plugin) Prefix() string { return "image" }

func (p *Plugin) HasServices() bool { return false }

func (p *Plugin) Nodes() []api.NodeRegistration {
	return []api.NodeRegistration{
		{Descriptor: &resizeDescriptor{}, Factory: newResizeExecutor},
		{Descriptor: &cropDescriptor{}, Factory: newCropExecutor},
		{Descriptor: &watermarkDescriptor{}, Factory: newWatermarkExecutor},
		{Descriptor: &convertDescriptor{}, Factory: newConvertExecutor},
		{Descriptor: &thumbnailDescriptor{}, Factory: newThumbnailExecutor},
	}
}

func (p *Plugin) CreateService(_ map[string]any) (any, error) { return nil, nil }
func (p *Plugin) HealthCheck(_ any) error                     { return nil }
func (p *Plugin) Shutdown(_ any) error                        { return nil }
