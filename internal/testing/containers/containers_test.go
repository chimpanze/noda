//go:build integration

package containers

import (
	"net/http"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestStartPostgres(t *testing.T) {
	url := StartPostgres(t)
	db, err := gorm.Open(postgres.Open(url), &gorm.Config{})
	require.NoError(t, err)
	var one int
	require.NoError(t, db.Raw("SELECT 1").Scan(&one).Error)
	require.Equal(t, 1, one)
}

func TestStartRedis(t *testing.T) {
	url := StartRedis(t)
	opts, err := redis.ParseURL(url)
	require.NoError(t, err)
	client := redis.NewClient(opts)
	defer client.Close()
	require.NoError(t, client.Ping(t.Context()).Err())
}

func TestStartMailpit(t *testing.T) {
	host, port, apiBase := StartMailpit(t)
	require.NotEmpty(t, host)
	require.Positive(t, port)
	require.Contains(t, apiBase, "http://")
	resp, err := http.Get(apiBase + "/api/v1/messages")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}
