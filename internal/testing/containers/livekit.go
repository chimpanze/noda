//go:build integration

package containers

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// StartLiveKit starts a LiveKit dev server (placeholder keys devkey/secret).
// --bind 0.0.0.0 is required: dev mode otherwise listens on 127.0.0.1 inside
// the container and the mapped port never answers.
func StartLiveKit(t testing.TB) (url, apiKey, apiSecret string) {
	t.Helper()
	ctx := context.Background()

	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "livekit/livekit-server:v1.9",
			Cmd:          []string{"--dev", "--bind", "0.0.0.0"},
			ExposedPorts: []string{"7880/tcp"},
			WaitingFor: wait.ForHTTP("/").WithPort("7880/tcp").
				WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	skipIfNoDocker(t, err)
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })

	host, err := ctr.Host(ctx)
	if err != nil {
		t.Fatalf("livekit host: %v", err)
	}
	port, err := ctr.MappedPort(ctx, "7880/tcp")
	if err != nil {
		t.Fatalf("livekit port: %v", err)
	}
	return fmt.Sprintf("ws://%s:%d", host, port.Num()), "devkey", "secret"
}
