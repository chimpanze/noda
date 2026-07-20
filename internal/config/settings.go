package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// Server setting readers. Values under "server" may arrive as JSON numbers or
// as strings produced by $env() resolution (internal/secrets/resolve.go always
// substitutes strings), so numeric settings accept both forms. Invalid values
// are errors, never silent fallbacks.

func serverSection(root map[string]any) (map[string]any, bool) {
	m, ok := root["server"].(map[string]any)
	return m, ok
}

// ServerInt reads server.<key> as an integer, accepting a JSON number or a
// numeric string. ok is false when the server section or key is absent.
func ServerInt(root map[string]any, key string) (int, bool, error) {
	m, ok := serverSection(root)
	if !ok {
		return 0, false, nil
	}
	v, ok := m[key]
	if !ok {
		return 0, false, nil
	}
	switch n := v.(type) {
	case float64:
		return int(n), true, nil
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(n))
		if err != nil {
			return 0, false, fmt.Errorf("server.%s: %q is not an integer", key, n)
		}
		return i, true, nil
	default:
		return 0, false, fmt.Errorf("server.%s: expected integer or string, got %T", key, v)
	}
}

// ServerDuration reads server.<key> as a Go duration string.
// ok is false when the server section or key is absent.
func ServerDuration(root map[string]any, key string) (time.Duration, bool, error) {
	m, ok := serverSection(root)
	if !ok {
		return 0, false, nil
	}
	v, ok := m[key]
	if !ok {
		return 0, false, nil
	}
	s, isStr := v.(string)
	if !isStr {
		return 0, false, fmt.Errorf("server.%s: expected duration string, got %T", key, v)
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, false, fmt.Errorf("server.%s: invalid duration %q: %v", key, s, err)
	}
	return d, true, nil
}

// TrustProxy is the parsed server.trust_proxy block. A nil *TrustProxy means
// the feature is off (block absent or enabled: false).
type TrustProxy struct {
	Proxies   []string
	Loopback  bool
	LinkLocal bool
	Private   bool
	Header    string
}

// ServerTrustProxy parses and validates server.trust_proxy. Fiber itself only
// warn-logs invalid proxy entries and silently trusts nothing when the set is
// empty, so both are hard errors here.
func ServerTrustProxy(root map[string]any) (*TrustProxy, error) {
	m, ok := serverSection(root)
	if !ok {
		return nil, nil
	}
	raw, ok := m["trust_proxy"]
	if !ok {
		return nil, nil
	}
	cfg, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("server.trust_proxy: expected object, got %T", raw)
	}
	if enabled, _ := cfg["enabled"].(bool); !enabled {
		return nil, nil
	}
	tp := &TrustProxy{Header: "X-Forwarded-For"}
	if h, ok := cfg["header"].(string); ok && h != "" {
		tp.Header = h
	}
	tp.Loopback, _ = cfg["loopback"].(bool)
	tp.LinkLocal, _ = cfg["link_local"].(bool)
	tp.Private, _ = cfg["private"].(bool)
	if rawList, ok := cfg["proxies"].([]any); ok {
		for i, item := range rawList {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("server.trust_proxy.proxies[%d]: expected string, got %T", i, item)
			}
			if strings.Contains(s, "/") {
				if _, _, err := net.ParseCIDR(s); err != nil {
					return nil, fmt.Errorf("server.trust_proxy.proxies[%d]: invalid CIDR %q", i, s)
				}
			} else if net.ParseIP(s) == nil {
				return nil, fmt.Errorf("server.trust_proxy.proxies[%d]: invalid IP %q", i, s)
			}
			tp.Proxies = append(tp.Proxies, s)
		}
	}
	if len(tp.Proxies) == 0 && !tp.Loopback && !tp.LinkLocal && !tp.Private {
		return nil, fmt.Errorf("server.trust_proxy: enabled but trusts nothing — set proxies or one of loopback/link_local/private")
	}
	return tp, nil
}

// OpenAPISettings is the parsed server.openapi block.
type OpenAPISettings struct {
	Enabled  bool
	Docs     bool
	Path     string
	DocsPath string
}

// ServerOpenAPI parses and validates server.openapi. It always returns a
// fully-defaulted *OpenAPISettings (never nil); err is non-nil only when a
// path is malformed. When the block is absent, Enabled is false and the rest
// carry defaults (Docs true, Path "/openapi.json", DocsPath "/docs").
func ServerOpenAPI(root map[string]any) (*OpenAPISettings, error) {
	s := &OpenAPISettings{Enabled: false, Docs: true, Path: "/openapi.json", DocsPath: "/docs"}
	m, ok := serverSection(root)
	if !ok {
		return s, nil
	}
	raw, ok := m["openapi"]
	if !ok {
		return s, nil
	}
	oa, ok := raw.(map[string]any)
	if !ok {
		return s, fmt.Errorf("server.openapi: expected object, got %T", raw)
	}
	if v, ok := oa["enabled"].(bool); ok {
		s.Enabled = v
	}
	if v, ok := oa["docs"].(bool); ok {
		s.Docs = v
	}
	if v, ok := oa["path"].(string); ok && v != "" {
		s.Path = v
	}
	if v, ok := oa["docs_path"].(string); ok && v != "" {
		s.DocsPath = v
	}
	if !strings.HasPrefix(s.Path, "/") {
		return s, fmt.Errorf("server.openapi.path: %q must start with \"/\"", s.Path)
	}
	if !strings.HasPrefix(s.DocsPath, "/") {
		return s, fmt.Errorf("server.openapi.docs_path: %q must start with \"/\"", s.DocsPath)
	}
	if s.Path == s.DocsPath {
		return s, fmt.Errorf("server.openapi: path and docs_path must differ (both %q)", s.Path)
	}
	return s, nil
}

// OpenAPIConfig returns the parsed server.openapi settings with defaults
// materialized. Boot validation (ValidateCrossRefs) already rejected malformed
// configs, so any parse error here is ignored and the defaulted settings win.
func (rc *ResolvedConfig) OpenAPIConfig() *OpenAPISettings {
	s, _ := ServerOpenAPI(rc.Root)
	return s
}
