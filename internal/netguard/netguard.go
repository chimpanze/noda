// Package netguard provides outbound-network policy checks: blocks workflows
// from dialing private, link-local, loopback, and cloud-metadata addresses
// unless explicitly opted in.
package netguard

import (
	"context"
	"errors"
	"fmt"
	"net"
)

// Policy controls which destinations a host is allowed to dial.
type Policy struct {
	// AllowPrivateNetworks, if true, lifts the default deny on RFC1918,
	// loopback, link-local, IPv6 unique-local, and CGN ranges. The two
	// metadata IPs (169.254.169.254, 100.100.100.200) remain blocked.
	AllowPrivateNetworks bool

	// AllowedHosts is an exact-match bypass list for hostnames whose
	// resolved IPs would otherwise be denied. Useful for naming a single
	// internal service without opening the entire RFC1918 range.
	// The two metadata IPs remain blocked even if their hostname is here.
	AllowedHosts []string

	// Resolver overrides the DNS resolver. Nil means net.DefaultResolver.
	Resolver *net.Resolver
}

// ErrDenied is returned when no resolved IP for a host is allowed.
var ErrDenied = errors.New("netguard: destination denied by policy")

// metadataIPs are uncircumventable: they remain blocked even when
// AllowPrivateNetworks is true or AllowedHosts contains the hostname.
var metadataIPs = []net.IP{
	net.ParseIP("169.254.169.254"), // AWS, GCP, Azure, DO, Oracle, IBM, OpenStack
	net.ParseIP("100.100.100.200"), // Alibaba Cloud
}

// privateBlocks are denied unless AllowPrivateNetworks=true.
var privateBlocks = mustParseCIDRs([]string{
	"127.0.0.0/8",    // IPv4 loopback
	"::1/128",        // IPv6 loopback
	"169.254.0.0/16", // IPv4 link-local
	"fe80::/10",      // IPv6 link-local
	"10.0.0.0/8",     // RFC1918
	"172.16.0.0/12",  // RFC1918
	"192.168.0.0/16", // RFC1918
	"fc00::/7",       // IPv6 unique-local
	"100.64.0.0/10",  // RFC 6598 CGN
})

// alwaysDenied are absolute regardless of policy.
var alwaysDenied = mustParseCIDRs([]string{
	"0.0.0.0/32", // Unspecified IPv4
})

func mustParseCIDRs(cidrs []string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, s := range cidrs {
		_, n, err := net.ParseCIDR(s)
		if err != nil {
			panic(fmt.Sprintf("netguard: bad CIDR %q: %v", s, err))
		}
		out = append(out, n)
	}
	return out
}

func ipInBlocks(ip net.IP, blocks []*net.IPNet) bool {
	for _, b := range blocks {
		if b.Contains(ip) {
			return true
		}
	}
	return false
}

func ipIsMetadata(ip net.IP) bool {
	for _, m := range metadataIPs {
		if m.Equal(ip) {
			return true
		}
	}
	return false
}

// CheckHost resolves host (a bare hostname or IP literal — no port) and
// returns the first IP that is allowed by the policy. The caller MUST dial
// the returned IP literal directly to defeat DNS rebinding.
func (p Policy) CheckHost(ctx context.Context, host string) (net.IP, error) {
	resolver := p.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	return p.checkHostWithLookup(ctx, host, resolver.LookupIPAddr)
}

// lookupFn matches resolver.LookupIPAddr; injected for tests.
type lookupFn func(ctx context.Context, host string) ([]net.IPAddr, error)

func (p Policy) checkHostWithLookup(ctx context.Context, host string, lookup lookupFn) (net.IP, error) {
	// If the AllowedHosts list contains this hostname, skip the
	// private-network deny but still apply metadata + always-denied checks.
	hostAllowed := false
	for _, h := range p.AllowedHosts {
		if h == host {
			hostAllowed = true
			break
		}
	}

	addrs, err := lookup(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("netguard: resolve %q: %w", host, err)
	}

	for _, a := range addrs {
		ip := a.IP
		if ipIsMetadata(ip) {
			continue
		}
		if ipInBlocks(ip, alwaysDenied) {
			continue
		}
		if !p.AllowPrivateNetworks && !hostAllowed && ipInBlocks(ip, privateBlocks) {
			continue
		}
		return ip, nil
	}

	return nil, fmt.Errorf("%w: host %q resolved to no allowed addresses", ErrDenied, host)
}

// ipDenied returns true if the IP must not be dialed under this policy.
func (p Policy) ipDenied(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ipIsMetadata(ip) {
		return true
	}
	if ipInBlocks(ip, alwaysDenied) {
		return true
	}
	if !p.AllowPrivateNetworks && ipInBlocks(ip, privateBlocks) {
		return true
	}
	return false
}
