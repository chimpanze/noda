//go:build integration

package containers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartLiveKit(t *testing.T) {
	url, key, secret := StartLiveKit(t)
	assert.Equal(t, "devkey", key)
	assert.Equal(t, "secret", secret)

	httpURL := strings.Replace(url, "ws://", "http://", 1)
	resp, err := http.Get(httpURL)
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)

	resp, err = http.Post(httpURL+"/twirp/livekit.RoomService/ListRooms", "application/json", strings.NewReader("{}"))
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, 401, resp.StatusCode, "unauthenticated twirp call must be rejected")
}
