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
