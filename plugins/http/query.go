package http

import (
	"fmt"
	"net/url"
)

// encodeQuery turns an already-resolved `query` config value into a URL-encoded
// query string with no leading "?". It returns "" when the value contributes no
// parameters, so a caller can tell "append nothing" from "append something"
// without re-inspecting the map.
//
// It lives in its own file because doRequest declares a local named `url`,
// which shadows net/url inside that function.
//
// Encoding is url.Values.Encode: RFC 3986 percent-encoding, keys sorted, and a
// space rendered as "+" (application/x-www-form-urlencoded). The sort is what
// makes output deterministic — the property #450 and #460 were both about.
func encodeQuery(raw any) (string, error) {
	if raw == nil {
		return "", nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return "", fmt.Errorf("query must resolve to an object, got %T", raw)
	}

	values := url.Values{}
	for k, v := range m {
		switch val := v.(type) {
		case nil:
			// A null value drops the key entirely, so an optional parameter
			// such as a pagination cursor needs no conditional at the call
			// site. Distinguishing absent from empty is deliberately not
			// supported; see the spec's "Values" decision.
		case []any:
			for _, el := range val {
				if el == nil {
					continue
				}
				s, err := queryScalar(k, el)
				if err != nil {
					return "", err
				}
				values.Add(k, s)
			}
		default:
			s, err := queryScalar(k, val)
			if err != nil {
				return "", err
			}
			values.Add(k, s)
		}
	}
	return values.Encode(), nil
}

// queryScalar stringifies one query value, rejecting the shapes that have no
// sane query-string encoding. Scalars are formatted with %v, matching what
// plugin.ResolveHeaders already does for header values.
func queryScalar(key string, v any) (string, error) {
	switch v.(type) {
	case map[string]any, []any:
		return "", fmt.Errorf("query value for %q must be a scalar or array of scalars", key)
	}
	return fmt.Sprintf("%v", v), nil
}
