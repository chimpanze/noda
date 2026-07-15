// Command groundtruth dumps the runtime's self-description (node types,
// schemas, expression functions) as JSON for the docs verification campaign.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/pkg/api"
	authplugin "github.com/chimpanze/noda/plugins/auth"
	cacheplugin "github.com/chimpanze/noda/plugins/cache"
	"github.com/chimpanze/noda/plugins/core/control"
	"github.com/chimpanze/noda/plugins/core/event"
	coreoidc "github.com/chimpanze/noda/plugins/core/oidc"
	"github.com/chimpanze/noda/plugins/core/response"
	coresse "github.com/chimpanze/noda/plugins/core/sse"
	corestorage "github.com/chimpanze/noda/plugins/core/storage"
	"github.com/chimpanze/noda/plugins/core/transform"
	"github.com/chimpanze/noda/plugins/core/upload"
	"github.com/chimpanze/noda/plugins/core/util"
	corewasm "github.com/chimpanze/noda/plugins/core/wasm"
	"github.com/chimpanze/noda/plugins/core/workflow"
	corews "github.com/chimpanze/noda/plugins/core/ws"
	dbplugin "github.com/chimpanze/noda/plugins/db"
	emailplugin "github.com/chimpanze/noda/plugins/email"
	httpplugin "github.com/chimpanze/noda/plugins/http"
	livekitplugin "github.com/chimpanze/noda/plugins/livekit"
	pubsubplugin "github.com/chimpanze/noda/plugins/pubsub"
	storageplugin "github.com/chimpanze/noda/plugins/storage"
	streamplugin "github.com/chimpanze/noda/plugins/stream"
)

// optionalPlugins is appended by the build-tagged plugins_image.go.
var optionalPlugins []api.Plugin

type nodeInfo struct {
	Type         string                    `json:"type"`
	Description  string                    `json:"description"`
	ConfigSchema map[string]any            `json:"config_schema,omitempty"`
	Outputs      []string                  `json:"outputs"`
	ServiceDeps  map[string]api.ServiceDep `json:"service_deps,omitempty"`
	OutputData   map[string]string         `json:"output_data,omitempty"`
}

func allPlugins() []api.Plugin {
	plugins := []api.Plugin{
		&control.Plugin{}, &transform.Plugin{}, &util.Plugin{},
		&workflow.Plugin{}, &response.Plugin{}, &dbplugin.Plugin{},
		&cacheplugin.Plugin{}, &event.Plugin{}, &corestorage.Plugin{},
		&upload.Plugin{}, &httpplugin.Plugin{}, &emailplugin.Plugin{},
		&corews.Plugin{}, &coresse.Plugin{}, &corewasm.Plugin{},
		&coreoidc.Plugin{}, &livekitplugin.Plugin{},
		&authplugin.Plugin{}, &streamplugin.Plugin{},
		&pubsubplugin.Plugin{}, &storageplugin.Plugin{},
	}
	return append(plugins, optionalPlugins...)
}

func buildGroundTruth() ([]nodeInfo, []expr.FunctionInfo, error) {
	reg := registry.NewNodeRegistry()
	for _, p := range allPlugins() {
		if err := reg.RegisterFromPlugin(p); err != nil {
			return nil, nil, fmt.Errorf("register %T: %w", p, err)
		}
	}
	types := reg.AllTypes()
	sort.Strings(types)
	nodes := make([]nodeInfo, 0, len(types))
	for _, t := range types {
		desc, ok := reg.GetDescriptor(t)
		if !ok {
			continue
		}
		outputs, _ := reg.OutputsForType(t)
		nodes = append(nodes, nodeInfo{
			Type:         t,
			Description:  desc.Description(),
			ConfigSchema: desc.ConfigSchema(),
			Outputs:      outputs,
			ServiceDeps:  desc.ServiceDeps(),
			OutputData:   desc.OutputDescriptions(),
		})
	}
	funcs := expr.NewFunctionRegistry().RegisteredFunctions()
	return nodes, funcs, nil
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func main() {
	outDir := ".verification/ground-truth"
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	nodes, funcs, err := buildGroundTruth()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := writeJSON(outDir+"/nodes.json", nodes); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := writeJSON(outDir+"/functions.json", funcs); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("dumped %d node types, %d functions to %s\n", len(nodes), len(funcs), outDir)
}
