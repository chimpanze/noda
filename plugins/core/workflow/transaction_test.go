package workflow

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestRun_DatabaseServiceDepOptional(t *testing.T) {
	desc := &runDescriptor{}
	deps := desc.ServiceDeps()
	// Database slot exists but is not required (only needed when transaction: true)
	assert.NotNil(t, deps)
	dbDep, ok := deps["database"]
	assert.True(t, ok)
	assert.Equal(t, "db", dbDep.Prefix)
	assert.False(t, dbDep.Required, "database dep should be optional")
}

func TestRun_ConfigSchemaAcceptsTransaction(t *testing.T) {
	desc := &runDescriptor{}
	schema := desc.ConfigSchema()
	props := schema["properties"].(map[string]any)
	_, hasTransaction := props["transaction"]
	assert.True(t, hasTransaction, "config schema should accept transaction field")
}

func TestRun_TransactionTrueConfig(t *testing.T) {
	config := map[string]any{
		"workflow":    "sub-wf",
		"transaction": true,
	}

	txn, ok := config["transaction"].(bool)
	assert.True(t, ok)
	assert.True(t, txn)
}

// --- Transaction execution tests ---

type mockTxExecCtx struct{}

func (m *mockTxExecCtx) Input() any          { return nil }
func (m *mockTxExecCtx) Auth() *api.AuthData { return nil }
func (m *mockTxExecCtx) Trigger() api.TriggerData {
	return api.TriggerData{Type: "test", Timestamp: time.Now(), TraceID: "tx-test"}
}
func (m *mockTxExecCtx) Resolve(expr string) (any, error)                           { return expr, nil }
func (m *mockTxExecCtx) ResolveWithVars(expr string, _ map[string]any) (any, error) { return expr, nil }
func (m *mockTxExecCtx) Log(_ string, _ string, _ map[string]any)                   {}

// mockTransactionalRunner implements TransactionalRunner for testing.
type mockTransactionalRunner struct {
	runFunc        func(ctx context.Context, workflowID string, input any, parentCtx api.ExecutionContext) (string, any, error)
	runWithSvcFunc func(ctx context.Context, workflowID string, input any, parentCtx api.ExecutionContext, overrides map[string]any) (string, any, error)
}

func (m *mockTransactionalRunner) RunSubWorkflow(ctx context.Context, workflowID string, input any, parentCtx api.ExecutionContext) (string, any, error) {
	if m.runFunc != nil {
		return m.runFunc(ctx, workflowID, input, parentCtx)
	}
	return "success", nil, nil
}

func (m *mockTransactionalRunner) RunSubWorkflowWithServices(ctx context.Context, workflowID string, input any, parentCtx api.ExecutionContext, overrides map[string]any) (string, any, error) {
	if m.runWithSvcFunc != nil {
		return m.runWithSvcFunc(ctx, workflowID, input, parentCtx, overrides)
	}
	return "success", nil, nil
}

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	_, err = sqlDB.Exec(`CREATE TABLE items (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL)`)
	require.NoError(t, err)
	return db
}

func TestRun_TransactionCommitOnSuccess(t *testing.T) {
	db := newTestDB(t)

	runner := &mockTransactionalRunner{
		runWithSvcFunc: func(ctx context.Context, _ string, _ any, _ api.ExecutionContext, overrides map[string]any) (string, any, error) {
			// The override should contain a *gorm.DB transaction
			txDB, ok := overrides["database"].(*gorm.DB)
			require.True(t, ok)
			// Insert a record using the transaction
			txDB.WithContext(ctx).Exec("INSERT INTO items (name) VALUES (?)", "committed-item")
			return "success", nil, nil
		},
	}

	exec := &RunExecutor{Runner: runner, outputs: []string{"success", "error"}}
	nCtx := &mockTxExecCtx{}

	config := map[string]any{
		"workflow":    "sub-wf",
		"transaction": true,
	}
	services := map[string]any{"database": db}

	output, _, err := exec.Execute(context.Background(), nCtx, config, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	// Verify the record was committed
	var count int64
	db.Raw("SELECT COUNT(*) FROM items WHERE name = ?", "committed-item").Scan(&count)
	assert.Equal(t, int64(1), count)
}

func TestRun_TransactionRollbackOnFailure(t *testing.T) {
	db := newTestDB(t)

	runner := &mockTransactionalRunner{
		runWithSvcFunc: func(ctx context.Context, _ string, _ any, _ api.ExecutionContext, overrides map[string]any) (string, any, error) {
			txDB := overrides["database"].(*gorm.DB)
			txDB.WithContext(ctx).Exec("INSERT INTO items (name) VALUES (?)", "rollback-item")
			return "", nil, fmt.Errorf("sub-workflow failed")
		},
	}

	exec := &RunExecutor{Runner: runner, outputs: []string{"success", "error"}}
	nCtx := &mockTxExecCtx{}

	config := map[string]any{
		"workflow":    "sub-wf",
		"transaction": true,
	}
	services := map[string]any{"database": db}

	_, _, err := exec.Execute(context.Background(), nCtx, config, services)
	require.Error(t, err)

	// Verify the record was rolled back
	var count int64
	db.Raw("SELECT COUNT(*) FROM items WHERE name = ?", "rollback-item").Scan(&count)
	assert.Equal(t, int64(0), count)
}

func TestRun_TransactionMultipleInserts(t *testing.T) {
	db := newTestDB(t)

	runner := &mockTransactionalRunner{
		runWithSvcFunc: func(ctx context.Context, _ string, _ any, _ api.ExecutionContext, overrides map[string]any) (string, any, error) {
			txDB := overrides["database"].(*gorm.DB)
			txDB.WithContext(ctx).Exec("INSERT INTO items (name) VALUES (?)", "item-1")
			txDB.WithContext(ctx).Exec("INSERT INTO items (name) VALUES (?)", "item-2")
			txDB.WithContext(ctx).Exec("INSERT INTO items (name) VALUES (?)", "item-3")
			return "success", nil, nil
		},
	}

	exec := &RunExecutor{Runner: runner, outputs: []string{"success", "error"}}
	nCtx := &mockTxExecCtx{}

	config := map[string]any{
		"workflow":    "sub-wf",
		"transaction": true,
	}
	services := map[string]any{"database": db}

	_, _, err := exec.Execute(context.Background(), nCtx, config, services)
	require.NoError(t, err)

	var count int64
	db.Raw("SELECT COUNT(*) FROM items").Scan(&count)
	assert.Equal(t, int64(3), count)
}

func TestRun_TransactionMultipleInsertsRollback(t *testing.T) {
	db := newTestDB(t)

	runner := &mockTransactionalRunner{
		runWithSvcFunc: func(ctx context.Context, _ string, _ any, _ api.ExecutionContext, overrides map[string]any) (string, any, error) {
			txDB := overrides["database"].(*gorm.DB)
			txDB.WithContext(ctx).Exec("INSERT INTO items (name) VALUES (?)", "item-1")
			txDB.WithContext(ctx).Exec("INSERT INTO items (name) VALUES (?)", "item-2")
			return "", nil, fmt.Errorf("fail after two inserts")
		},
	}

	exec := &RunExecutor{Runner: runner, outputs: []string{"success", "error"}}
	nCtx := &mockTxExecCtx{}

	config := map[string]any{
		"workflow":    "sub-wf",
		"transaction": true,
	}
	services := map[string]any{"database": db}

	_, _, err := exec.Execute(context.Background(), nCtx, config, services)
	require.Error(t, err)

	// All inserts should be rolled back
	var count int64
	db.Raw("SELECT COUNT(*) FROM items").Scan(&count)
	assert.Equal(t, int64(0), count)
}

func TestRun_TransactionMissingDatabaseService(t *testing.T) {
	runner := &mockTransactionalRunner{}
	exec := &RunExecutor{Runner: runner, outputs: []string{"success", "error"}}
	nCtx := &mockTxExecCtx{}

	config := map[string]any{
		"workflow":    "sub-wf",
		"transaction": true,
	}

	_, _, err := exec.Execute(context.Background(), nCtx, config, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transaction requires a database service")
}

func TestRun_TransactionRunnerNotSupported(t *testing.T) {
	// Use a basic SubWorkflowRunner that doesn't implement TransactionalRunner
	runner := &basicRunner{}
	exec := &RunExecutor{Runner: runner, outputs: []string{"success", "error"}}
	nCtx := &mockTxExecCtx{}

	config := map[string]any{
		"workflow":    "sub-wf",
		"transaction": true,
	}
	db := newTestDB(t)
	services := map[string]any{"database": db}

	_, _, err := exec.Execute(context.Background(), nCtx, config, services)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "runner does not support transactions")
}

// basicRunner only implements SubWorkflowRunner (not TransactionalRunner).
type basicRunner struct{}

func (r *basicRunner) RunSubWorkflow(_ context.Context, _ string, _ any, _ api.ExecutionContext) (string, any, error) {
	return "success", nil, nil
}

func TestRun_NestedTransactionSavepoint(t *testing.T) {
	db := newTestDB(t)

	runner := &mockTransactionalRunner{
		runWithSvcFunc: func(ctx context.Context, _ string, _ any, _ api.ExecutionContext, overrides map[string]any) (string, any, error) {
			txDB := overrides["database"].(*gorm.DB)
			// Insert in outer transaction
			txDB.WithContext(ctx).Exec("INSERT INTO items (name) VALUES (?)", "outer")

			// Simulate nested transaction (savepoint)
			nestedErr := txDB.WithContext(ctx).Transaction(func(innerTx *gorm.DB) error {
				innerTx.Exec("INSERT INTO items (name) VALUES (?)", "inner")
				return nil // commit savepoint
			})
			require.NoError(t, nestedErr)

			return "success", nil, nil
		},
	}

	exec := &RunExecutor{Runner: runner, outputs: []string{"success", "error"}}
	nCtx := &mockTxExecCtx{}

	config := map[string]any{
		"workflow":    "sub-wf",
		"transaction": true,
	}
	services := map[string]any{"database": db}

	_, _, err := exec.Execute(context.Background(), nCtx, config, services)
	require.NoError(t, err)

	var count int64
	db.Raw("SELECT COUNT(*) FROM items").Scan(&count)
	assert.Equal(t, int64(2), count)
}
