package util

import (
	"context"
	"regexp"
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var uuidRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestUUID_ValidFormat(t *testing.T) {
	executor := newUUIDExecutor(nil)
	execCtx := engine.NewExecutionContext()

	output, data, err := executor.Execute(context.Background(), execCtx, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Regexp(t, uuidRegex, data.(string))
}

func TestUUID_Unique(t *testing.T) {
	executor := newUUIDExecutor(nil)
	execCtx := engine.NewExecutionContext()

	_, data1, _ := executor.Execute(context.Background(), execCtx, nil, nil)
	_, data2, _ := executor.Execute(context.Background(), execCtx, nil, nil)
	assert.NotEqual(t, data1, data2)
}

func TestUUID_Descriptor(t *testing.T) {
	d := &uuidDescriptor{}
	assert.Equal(t, "uuid", d.Name())
	assert.Nil(t, d.ServiceDeps())
	assert.Nil(t, d.ConfigSchema())
}

func TestUUID_Outputs(t *testing.T) {
	executor := newUUIDExecutor(nil)
	assert.Equal(t, []string{"success", "error"}, executor.Outputs())
}
