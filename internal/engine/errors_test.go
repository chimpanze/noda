package engine

import (
	"errors"
	"fmt"
	"testing"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNodeExecutionError_Error(t *testing.T) {
	inner := fmt.Errorf("resolve: evaluation error")
	err := &NodeExecutionError{
		NodeID:         "respond",
		NodeType:       "response.json",
		Err:            inner,
		AvailableNodes: []string{"create_player", "hash_password"},
	}

	msg := err.Error()
	assert.Contains(t, msg, "respond")
	assert.Contains(t, msg, "response.json")
	assert.Contains(t, msg, "evaluation error")
	assert.Contains(t, msg, "create_player")
	assert.Contains(t, msg, "hash_password")
}

func TestNodeExecutionError_Unwrap(t *testing.T) {
	valErr := &api.ValidationError{Field: "name", Message: "required"}
	nodeErr := &NodeExecutionError{
		NodeID:   "validate",
		NodeType: "transform.validate",
		Err:      valErr,
	}

	// errors.As should find the inner ValidationError
	var target *api.ValidationError
	require.True(t, errors.As(nodeErr, &target))
	assert.Equal(t, "name", target.Field)

	// errors.Unwrap should return the inner error
	assert.Equal(t, valErr, errors.Unwrap(nodeErr))
}
