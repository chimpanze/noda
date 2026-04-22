package http

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/chimpanze/noda/internal/netguard"
)

// newTransport returns an http.Transport whose DialContext enforces
// the supplied netguard.Policy. base may be nil (defaults to a fresh
// transport with reasonable defaults).
func newTransport(policy netguard.Policy, base *http.Transport) *http.Transport {
	if base == nil {
		base = &http.Transport{
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		}
	}

	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	base.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("http: parse address %q: %w", address, err)
		}

		// If host is already an IP literal, skip resolution but still check it.
		if ip := net.ParseIP(host); ip != nil {
			if policy.IPDeniedExported(ip) {
				return nil, fmt.Errorf("%w: %s", netguard.ErrDenied, ip)
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		}

		ip, err := policy.CheckHost(ctx, host)
		if err != nil {
			return nil, err
		}
		// Dial the IP literal directly — defeats DNS rebinding.
		return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
	}

	return base
}
