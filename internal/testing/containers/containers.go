//go:build integration

// Package containers provides testcontainers-backed helpers for external-service
// end-to-end tests. Each helper starts one container, registers cleanup, and
// skips the test if Docker is unavailable.
package containers

import (
	"context"
	"fmt"
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
	return host, int(smtpPort.Num()), apiBase
}
