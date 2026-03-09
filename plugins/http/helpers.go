package http

import (
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
)

var httpServiceDeps = map[string]api.ServiceDep{
	"client": {Prefix: "http", Required: true},
}

func getHTTPService(services map[string]any) (*Service, error) {
	return plugin.GetService[*Service](services, "client")
}

func resolveHeaders(nCtx api.ExecutionContext, config map[string]any) (map[string]string, error) {
	raw, ok := config["headers"]
	if !ok {
		return nil, nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("field \"headers\" must be a map")
	}
	result := make(map[string]string, len(m))
	for k, v := range m {
		expr, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("header %q value must be a string", k)
		}
		val, err := nCtx.Resolve(expr)
		if err != nil {
			return nil, fmt.Errorf("resolve header %q: %w", k, err)
		}
		s, ok := val.(string)
		if !ok {
			return nil, fmt.Errorf("header %q resolved to %T, expected string", k, val)
		}
		result[k] = s
	}
	return result, nil
}
