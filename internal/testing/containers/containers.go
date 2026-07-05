//go:build integration

// Package containers provides testcontainers-backed helpers for external-service
// end-to-end tests. Each helper starts one container, registers cleanup, and
// skips the test if Docker is unavailable.
package containers

import (
	"context"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"
)

func skipIfNoDocker(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Skipf("skipping: cannot start container (is Docker running?): %v", err)
	}
}

// StartPostgres starts a postgres:17-alpine container and returns a gorm-ready URL.
func StartPostgres(t testing.TB) string {
	t.Helper()
	ctx := context.Background()
	ctr, err := postgres.Run(ctx, "postgres:17-alpine",
		postgres.WithDatabase("noda"),
		postgres.WithUsername("noda"),
		postgres.WithPassword("noda"),
		postgres.BasicWaitStrategies(),
	)
	skipIfNoDocker(t, err)
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })

	url, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("postgres connection string: %v", err)
	}
	return url
}

// StartRedis starts a redis:7-alpine container and returns a redis:// URL.
func StartRedis(t testing.TB) string {
	t.Helper()
	ctx := context.Background()
	ctr, err := tcredis.Run(ctx, "redis:7-alpine")
	skipIfNoDocker(t, err)
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })

	url, err := ctr.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("redis connection string: %v", err)
	}
	return url
}

// StartMailpit starts a Mailpit container and returns its SMTP host/port and HTTP API base URL.
func StartMailpit(t testing.TB) (string, int, string) {
	t.Helper()
	ctx := context.Background()
	ctr, err := testcontainers.Run(ctx, "axllent/mailpit:v1.20",
		testcontainers.WithExposedPorts("1025/tcp", "8025/tcp"),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("1025/tcp").WithStartupTimeout(30*time.Second),
		),
	)
	skipIfNoDocker(t, err)
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })

	host, err := ctr.Host(ctx)
	if err != nil {
		t.Fatalf("mailpit host: %v", err)
	}
	smtpPort, err := ctr.MappedPort(ctx, "1025/tcp")
	if err != nil {
		t.Fatalf("mailpit smtp port: %v", err)
	}
	apiPort, err := ctr.MappedPort(ctx, "8025/tcp")
	if err != nil {
		t.Fatalf("mailpit api port: %v", err)
	}
	apiBase := fmt.Sprintf("http://%s:%s", host, apiPort.Port())
	waitForSMTPBanner(t, fmt.Sprintf("%s:%d", host, int(smtpPort.Num())))
	return host, int(smtpPort.Num()), apiBase
}

// waitForSMTPBanner blocks until the SMTP server behind addr sends its "220"
// greeting. The ForListeningPort wait checks the host-mapped port, which
// docker-proxy accepts before the app inside is serving — dials in that window
// get "connect: EOF" (the TestEmailSend_Engine flake).
func waitForSMTPBanner(t testing.TB, addr string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err == nil {
			_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			buf := make([]byte, 3)
			_, rerr := io.ReadFull(conn, buf)
			_ = conn.Close()
			if rerr == nil && string(buf) == "220" {
				return
			}
			lastErr = rerr
		} else {
			lastErr = err
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("mailpit SMTP never became ready at %s: %v", addr, lastErr)
}
