package worker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The worker schema declares "concurrency": {"type": "integer"} with no bound,
// so an over-limit value passes file validation and aborts Start.
func TestValidateConfigs_RejectsConcurrencyOverMaximum(t *testing.T) {
	errs := ValidateConfigs([]WorkerConfig{
		{ID: "ingest", Concurrency: maxConcurrency + 1, SourceFile: "/proj/workers/ingest.json"},
	})

	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "ingest")
	assert.Contains(t, errs[0].Error(), "exceeds maximum")
	assert.Equal(t, "/proj/workers/ingest.json", errs[0].Config.SourceFile)
}

func TestValidateConfigs_AcceptsConcurrencyAtMaximum(t *testing.T) {
	assert.Empty(t, ValidateConfigs([]WorkerConfig{
		{ID: "ingest", Concurrency: maxConcurrency},
	}))
}

// Start applies max(Concurrency, 1), so zero and negative values are legal
// config meaning "one consumer" — validation must not reject them.
func TestValidateConfigs_AcceptsZeroAndNegativeConcurrency(t *testing.T) {
	assert.Empty(t, ValidateConfigs([]WorkerConfig{
		{ID: "a", Concurrency: 0},
		{ID: "b", Concurrency: -1},
	}))
}

func TestParseWorkerConfigs_RecordsSourceFile(t *testing.T) {
	configs := ParseWorkerConfigs(map[string]map[string]any{
		"/proj/workers/ingest.json": {"id": "ingest"},
	})

	require.Len(t, configs, 1)
	assert.Equal(t, "/proj/workers/ingest.json", configs[0].SourceFile)
}
