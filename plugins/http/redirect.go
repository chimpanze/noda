package http

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// buildCheckRedirect returns a CheckRedirect function for the given mode
// ("none" | "same_origin" | "strip_auth") and hop limit.
func buildCheckRedirect(mode string, max int) func(req *http.Request, via []*http.Request) error {
	switch mode {
	case "none":
		return func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
	case "same_origin":
		return func(req *http.Request, via []*http.Request) error {
			if len(via) >= max {
				return fmt.Errorf("http: max redirects (%d) exceeded", max)
			}
			if !sameOrigin(req.URL, via[0].URL) {
				return fmt.Errorf("http: cross-origin redirect denied: %s -> %s", originString(via[0].URL), originString(req.URL))
			}
			return nil
		}
	case "strip_auth":
		return func(req *http.Request, via []*http.Request) error {
			if len(via) >= max {
				return fmt.Errorf("http: max redirects (%d) exceeded", max)
			}
			if !sameOrigin(req.URL, via[len(via)-1].URL) {
				stripAuthHeaders(req.Header)
			}
			return nil
		}
	default:
		// Should never happen — CreateService validates this.
		return func(*http.Request, []*http.Request) error {
			return fmt.Errorf("http: unknown redirect mode %q", mode)
		}
	}
}

// sameOrigin reports whether two URLs share scheme, host, and port. Default
// ports (80 for http, 443 for https) are normalised before comparison.
func sameOrigin(a, b *url.URL) bool {
	if a == nil || b == nil {
		return false
	}
	return a.Scheme == b.Scheme && hostPortNormalised(a) == hostPortNormalised(b)
}

func hostPortNormalised(u *url.URL) string {
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		switch u.Scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		}
	}
	return host + ":" + port
}

func originString(u *url.URL) string {
	return u.Scheme + "://" + hostPortNormalised(u)
}

// authHeaderNames are stripped on cross-origin hops in strip_auth mode.
var authHeaderNames = []string{
	"Authorization",
	"Cookie",
	"Proxy-Authorization",
	"X-Api-Key",
	"X-Auth-Token",
}

func stripAuthHeaders(h http.Header) {
	for _, name := range authHeaderNames {
		h.Del(name)
	}
	// Also strip any X-*-Token / X-*-Key headers (case-insensitive on suffix).
	for k := range h {
		canon := http.CanonicalHeaderKey(k)
		if !strings.HasPrefix(canon, "X-") {
			continue
		}
		if strings.HasSuffix(canon, "-Token") || strings.HasSuffix(canon, "-Key") {
			h.Del(k)
		}
	}
}
