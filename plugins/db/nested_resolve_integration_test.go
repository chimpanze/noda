//go:build integration

package db

import (
	"encoding/json"
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/internal/testing/containers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupJSONCol builds a service with a table carrying a JSON/JSONB column.
func setupJSONCol(t *testing.T, driver string) (*registry.ServiceRegistry, *registry.NodeRegistry, *gorm.DB) {
	t.Helper()
	cfg := map[string]any{"driver": driver}
	colType := "jsonb"
	if driver == "postgres" {
		cfg["url"] = containers.StartPostgres(t)
	} else {
		cfg["path"] = t.TempDir() + "/t.db"
		colType = "text"
	}
	svc, err := (&Plugin{}).CreateService(cfg)
	require.NoError(t, err)
	gdb := svc.(*gorm.DB)

	require.NoError(t, gdb.Exec(
		`CREATE TABLE profiles (id integer PRIMARY KEY, metadata `+colType+`)`).Error)
	require.NoError(t, gdb.Exec(
		`INSERT INTO profiles (id, metadata) VALUES (1, '{}')`).Error)

	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("db", svc, nil))
	nodeReg := registry.NewNodeRegistry()
	require.NoError(t, nodeReg.RegisterFromPlugin(&Plugin{}))
	return svcReg, nodeReg, gdb
}

// A nested object in db.update's "data" must have its templated leaves
// evaluated before the column is written. Previously the nested map was copied
// verbatim and the literal "{{ ... }}" text landed in the database (#438).
func TestNestedResolve_UpdateJSONColumn_BothDrivers(t *testing.T) {
	for _, driver := range []string{"postgres", "sqlite"} {
		t.Run(driver, func(t *testing.T) {
			svcReg, nodeReg, gdb := setupJSONCol(t, driver)

			wf := engine.WorkflowConfig{
				ID: "wf",
				Nodes: map[string]engine.NodeConfig{
					"n1": {
						Type:     "db.update",
						Services: map[string]string{"database": "db"},
						Config: map[string]any{
							"table": "profiles",
							"where": map[string]any{"id": 1},
							"data": map[string]any{
								"metadata": map[string]any{
									"username": "{{ input.username }}",
									"nested":   map[string]any{"bio": "{{ input.bio }}"},
								},
							},
						},
					},
				},
			}
			graph, err := engine.Compile(wf, nodeReg)
			require.NoError(t, err)

			execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
				"username": "alice",
				"bio":      "hello world",
			}))
			require.NoError(t, engine.ExecuteGraph(t.Context(), graph, execCtx, svcReg, nodeReg))

			var raw string
			require.NoError(t, gdb.Raw(`SELECT metadata FROM profiles WHERE id = 1`).Scan(&raw).Error)

			var got map[string]any
			require.NoError(t, json.Unmarshal([]byte(raw), &got))

			assert.Equal(t, "alice", got["username"],
				"nested template must be evaluated, not written as literal text")
			nested, ok := got["nested"].(map[string]any)
			require.True(t, ok, "nested object should survive as an object")
			assert.Equal(t, "hello world", nested["bio"],
				"resolution must recurse to arbitrary depth")
		})
	}
}
