//go:build integration

package db

import (
	"errors"
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/internal/testing/containers"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupClassify(t *testing.T, driver string) (*registry.ServiceRegistry, *registry.NodeRegistry) {
	t.Helper()
	cfg := map[string]any{"driver": driver}
	if driver == "postgres" {
		cfg["url"] = containers.StartPostgres(t)
	} else {
		cfg["path"] = t.TempDir() + "/t.db"
	}
	svc, err := (&Plugin{}).CreateService(cfg)
	require.NoError(t, err)
	gdb := svc.(*gorm.DB)

	require.NoError(t, gdb.Exec(
		`CREATE TABLE cls (id integer PRIMARY KEY, age integer NOT NULL, email text UNIQUE)`).Error)
	require.NoError(t, gdb.Exec(
		`INSERT INTO cls (id, age, email) VALUES (1, 30, 'a@example.com')`).Error)

	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("db", svc, nil))
	nodeReg := registry.NewNodeRegistry()
	require.NoError(t, nodeReg.RegisterFromPlugin(&Plugin{}))
	return svcReg, nodeReg
}

func runOne(t *testing.T, svcReg *registry.ServiceRegistry, nodeReg *registry.NodeRegistry,
	nodeType string, config map[string]any) error {
	t.Helper()
	wf := engine.WorkflowConfig{
		ID: "wf",
		Nodes: map[string]engine.NodeConfig{
			"n1": {Type: nodeType, Services: map[string]string{"database": "db"}, Config: config},
		},
	}
	graph, err := engine.Compile(wf, nodeReg)
	require.NoError(t, err)
	return engine.ExecuteGraph(t.Context(), graph, engine.NewExecutionContext(), svcReg, nodeReg)
}

// Regression: this returned a raw 500 on SQLite because the old matcher
// looked for lowercase "unique constraint" but SQLite emits "UNIQUE
// constraint failed".
//
// The seed row is id=1. This insert supplies a distinct id=2 so that on
// Postgres (where "id" has no default) the row satisfies the NOT NULL/PK
// constraint on id and only the UNIQUE constraint on email can fire.
func TestClassify_UniqueViolation_BothDrivers(t *testing.T) {
	for _, driver := range []string{"postgres", "sqlite"} {
		t.Run(driver, func(t *testing.T) {
			svcReg, nodeReg := setupClassify(t, driver)
			err := runOne(t, svcReg, nodeReg, "db.create", map[string]any{
				"table": "cls",
				"data":  map[string]any{"id": 2, "age": 1, "email": "a@example.com"},
			})
			require.Error(t, err)
			var ce *api.ConflictError
			assert.True(t, errors.As(err, &ce), "want ConflictError, got %v", err)
		})
	}
}

// Regression: update.go had no conflict handling, so the same violation
// returned 500 from db.update and 409 from db.create.
//
// The created row uses id=3 with a fresh email, then the update tries to
// rename that email to the seed row's email — only the UNIQUE constraint on
// email is in play.
func TestClassify_UpdateUniqueViolation_BothDrivers(t *testing.T) {
	for _, driver := range []string{"postgres", "sqlite"} {
		t.Run(driver, func(t *testing.T) {
			svcReg, nodeReg := setupClassify(t, driver)
			require.NoError(t, runOne(t, svcReg, nodeReg, "db.create", map[string]any{
				"table": "cls",
				"data":  map[string]any{"id": 3, "age": 40, "email": "b@example.com"},
			}))
			err := runOne(t, svcReg, nodeReg, "db.update", map[string]any{
				"table": "cls",
				"where": map[string]any{"email": "b@example.com"},
				"data":  map[string]any{"email": "a@example.com"},
			})
			require.Error(t, err)
			var ce *api.ConflictError
			assert.True(t, errors.As(err, &ce), "want ConflictError, got %v", err)
		})
	}
}

// The inserted row uses a distinct id=4 and a fresh email so that only the
// omitted "age" (NOT NULL, no default) can violate a constraint.
func TestClassify_NotNullViolation_BothDrivers(t *testing.T) {
	for _, driver := range []string{"postgres", "sqlite"} {
		t.Run(driver, func(t *testing.T) {
			svcReg, nodeReg := setupClassify(t, driver)
			err := runOne(t, svcReg, nodeReg, "db.create", map[string]any{
				"table": "cls",
				"data":  map[string]any{"id": 4, "email": "c@example.com"},
			})
			require.Error(t, err)
			var ve *api.ValidationError
			assert.True(t, errors.As(err, &ve), "want ValidationError, got %v", err)
		})
	}
}

// Postgres only: SQLite's dynamic typing accepts a string in an integer
// column without error, so there is nothing to classify there.
func TestClassify_InvalidTextRepresentation_PostgresOnly(t *testing.T) {
	svcReg, nodeReg := setupClassify(t, "postgres")
	err := runOne(t, svcReg, nodeReg, "db.find", map[string]any{
		"table": "cls",
		"where": map[string]any{"age": "not-a-number"},
	})
	require.Error(t, err)
	var ve *api.ValidationError
	assert.True(t, errors.As(err, &ve), "want ValidationError, got %v", err)
}

// Class 42 is an author bug and must stay unmapped, i.e. a 500.
func TestClassify_UndefinedColumnStaysUnmapped(t *testing.T) {
	svcReg, nodeReg := setupClassify(t, "postgres")
	err := runOne(t, svcReg, nodeReg, "db.find", map[string]any{
		"table": "cls",
		"where": map[string]any{"nope": "x"},
	})
	require.Error(t, err)
	var ve *api.ValidationError
	var ce *api.ConflictError
	assert.False(t, errors.As(err, &ve))
	assert.False(t, errors.As(err, &ce))
}
