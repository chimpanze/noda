package scheduler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The runtime installs cron.WithSeconds(), so a five-field spec — the form
// every non-Go cron accepts — is rejected. This is the shape that made
// testdata/valid-project unbootable while `noda validate` called it clean.
func TestValidateSpecs_RejectsFiveFieldSpec(t *testing.T) {
	errs := ValidateSpecs([]ScheduleConfig{
		{ID: "cleanup", Cron: "0 */6 * * *", SourceFile: "/proj/schedules/cleanup.json"},
	})

	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "cleanup")
	assert.Contains(t, errs[0].Error(), "expected exactly 6 fields")
	assert.Equal(t, "/proj/schedules/cleanup.json", errs[0].Config.SourceFile)
}

func TestValidateSpecs_AcceptsSixFieldSpec(t *testing.T) {
	assert.Empty(t, ValidateSpecs([]ScheduleConfig{
		{ID: "cleanup", Cron: "0 0 */6 * * *"},
	}))
}

func TestValidateSpecs_AcceptsDescriptor(t *testing.T) {
	assert.Empty(t, ValidateSpecs([]ScheduleConfig{
		{ID: "nightly", Cron: "@daily"},
	}))
}

// A timezone prefix is part of the spec Start registers, so it is part of what
// gets validated.
func TestValidateSpecs_ValidatesWithTimezonePrefix(t *testing.T) {
	assert.Empty(t, ValidateSpecs([]ScheduleConfig{
		{ID: "tz", Cron: "0 0 3 * * *", Timezone: "Europe/Berlin"},
	}))
}

// Attribution: internal/startup needs the declaring file, and
// ParseScheduleConfigs is the only place that still knows it.
func TestParseScheduleConfigs_RecordsSourceFile(t *testing.T) {
	configs := ParseScheduleConfigs(map[string]map[string]any{
		"/proj/schedules/cleanup.json": {"id": "cleanup", "cron": "0 0 * * * *"},
	})

	require.Len(t, configs, 1)
	assert.Equal(t, "/proj/schedules/cleanup.json", configs[0].SourceFile)
}
