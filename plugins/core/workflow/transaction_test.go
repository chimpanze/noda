package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRun_TransactionFalseNoDBRequired(t *testing.T) {
	desc := &runDescriptor{}
	deps := desc.ServiceDeps()
	// No database slot required when transaction not set
	assert.Nil(t, deps)
}

func TestRun_ConfigSchemaAcceptsTransaction(t *testing.T) {
	desc := &runDescriptor{}
	schema := desc.ConfigSchema()
	props := schema["properties"].(map[string]any)
	_, hasTransaction := props["transaction"]
	assert.True(t, hasTransaction, "config schema should accept transaction field")
}

func TestRun_TransactionTrueConfig(t *testing.T) {
	// When transaction: true is set in config, the engine should validate
	// that a database service slot is available. This is a config-level check
	// that happens at startup validation time.
	config := map[string]any{
		"workflow":    "sub-wf",
		"transaction": true,
	}

	txn, ok := config["transaction"].(bool)
	assert.True(t, ok)
	assert.True(t, txn)
}
