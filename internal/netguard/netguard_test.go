package netguard

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIPDenied_Loopback(t *testing.T) {
	p := Policy{}
	assert.True(t, p.ipDenied(net.ParseIP("127.0.0.1")))
	assert.True(t, p.ipDenied(net.ParseIP("127.255.255.254")))
	assert.True(t, p.ipDenied(net.ParseIP("::1")))
}

func TestIPDenied_LinkLocal(t *testing.T) {
	p := Policy{}
	assert.True(t, p.ipDenied(net.ParseIP("169.254.0.1")))
	assert.True(t, p.ipDenied(net.ParseIP("169.254.169.254")))
	assert.True(t, p.ipDenied(net.ParseIP("fe80::1")))
}

func TestIPDenied_RFC1918(t *testing.T) {
	p := Policy{}
	for _, ip := range []string{"10.0.0.1", "10.255.255.254", "172.16.0.1", "172.31.255.254", "192.168.0.1", "192.168.255.254"} {
		assert.Truef(t, p.ipDenied(net.ParseIP(ip)), "expected %s denied", ip)
	}
}

func TestIPDenied_UniqueLocalV6(t *testing.T) {
	p := Policy{}
	assert.True(t, p.ipDenied(net.ParseIP("fc00::1")))
	assert.True(t, p.ipDenied(net.ParseIP("fd00::1")))
}

func TestIPDenied_CGN(t *testing.T) {
	p := Policy{}
	assert.True(t, p.ipDenied(net.ParseIP("100.64.0.1")))
	assert.True(t, p.ipDenied(net.ParseIP("100.100.100.200")))
}

func TestIPDenied_Unspecified(t *testing.T) {
	p := Policy{}
	assert.True(t, p.ipDenied(net.ParseIP("0.0.0.0")))
}

func TestIPDenied_PublicAllowed(t *testing.T) {
	p := Policy{}
	for _, ip := range []string{"8.8.8.8", "1.1.1.1", "93.184.216.34", "2606:4700:4700::1111"} {
		assert.Falsef(t, p.ipDenied(net.ParseIP(ip)), "expected %s allowed", ip)
	}
}

func TestIPDenied_AllowPrivateOpensRFC1918(t *testing.T) {
	p := Policy{AllowPrivateNetworks: true}
	assert.False(t, p.ipDenied(net.ParseIP("10.0.0.1")))
	assert.False(t, p.ipDenied(net.ParseIP("192.168.1.1")))
	assert.False(t, p.ipDenied(net.ParseIP("169.254.0.1")), "link-local should also open with AllowPrivateNetworks")
}

func TestIPDenied_MetadataIPsAlwaysBlocked(t *testing.T) {
	p := Policy{AllowPrivateNetworks: true}
	assert.True(t, p.ipDenied(net.ParseIP("169.254.169.254")), "AWS/GCP/Azure metadata must remain blocked")
	assert.True(t, p.ipDenied(net.ParseIP("100.100.100.200")), "Alibaba metadata must remain blocked")
}
