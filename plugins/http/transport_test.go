package http

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chimpanze/noda/internal/netguard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTransport_BlocksLoopbackByDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tp := newTransport(netguard.Policy{}, nil)
	client := &http.Client{Transport: tp, Timeout: 2 * time.Second}

	_, err := client.Get(srv.URL)
	require.Error(t, err)
	assert.ErrorIs(t, err, netguard.ErrDenied)
}

func TestNewTransport_AllowsLoopbackWhenAllowPrivate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tp := newTransport(netguard.Policy{AllowPrivateNetworks: true}, nil)
	client := &http.Client{Transport: tp, Timeout: 2 * time.Second}

	resp, err := client.Get(srv.URL)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestNewTransport_DialContextRejectsBeforeDial(t *testing.T) {
	// Direct IP literal in private range — checked without DNS.
	tp := newTransport(netguard.Policy{}, nil)
	client := &http.Client{Transport: tp, Timeout: 2 * time.Second}

	_, err := client.Get("http://10.0.0.1:80/")
	require.Error(t, err)
	assert.ErrorIs(t, err, netguard.ErrDenied)

	// Use _ = context.Background just to keep the import; the test above doesn't use it.
	_ = context.Background()
	_ = net.IPv4zero
}
