//go:build integration

package db

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/internal/testing/containers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupDB starts Postgres, builds the service + registries, and creates a fresh
// table unique to the test.
func setupDB(t *testing.T, table string) (*registry.ServiceRegistry, *registry.NodeRegistry, *gorm.DB) {
	t.Helper()
	url := containers.StartPostgres(t)

	svc, err := (&Plugin{}).CreateService(map[string]any{"driver": "postgres", "url": url})
	require.NoError(t, err)
	gdb := svc.(*gorm.DB)
	require.NoError(t, gdb.Exec(
		"CREATE TABLE "+table+" (id serial PRIMARY KEY, name text, email text UNIQUE)").Error)
	t.Cleanup(func() { gdb.Exec("DROP TABLE IF EXISTS " + table) })

	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("db", svc, nil))
	nodeReg := registry.NewNodeRegistry()
	require.NoError(t, nodeReg.RegisterFromPlugin(&Plugin{}))
	return svcReg, nodeReg, gdb
}

func runNode(t *testing.T, svcReg *registry.ServiceRegistry, nodeReg *registry.NodeRegistry,
	wf engine.WorkflowConfig, input map[string]any) *engine.ExecutionContextImpl {
	t.Helper()
	graph, err := engine.Compile(wf, nodeReg)
	require.NoError(t, err)
	execCtx := engine.NewExecutionContext(engine.WithInput(input))
	require.NoError(t, engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg))
	return execCtx
}

func TestDBCreateAndFind_Engine(t *testing.T) {
	svcReg, nodeReg, gdb := setupDB(t, "users_create")

	createWF := engine.WorkflowConfig{
		ID: "db-create",
		Nodes: map[string]engine.NodeConfig{
			"c": {
				Type:     "db.create",
				Services: map[string]string{"database": "db"},
				Config: map[string]any{
					"table": "users_create",
					"data":  map[string]any{"name": "Alice", "email": "alice@example.com"},
				},
			},
		},
	}
	execCtx := runNode(t, svcReg, nodeReg, createWF, nil)
	out, ok := execCtx.GetOutput("c")
	require.True(t, ok)
	row := out.(map[string]any)
	assert.Equal(t, "Alice", row["name"])
	assert.NotNil(t, row["id"])

	// Effect asserted directly against Postgres.
	var count int64
	require.NoError(t, gdb.Table("users_create").Where("email = ?", "alice@example.com").Count(&count).Error)
	assert.Equal(t, int64(1), count)

	findWF := engine.WorkflowConfig{
		ID: "db-find",
		Nodes: map[string]engine.NodeConfig{
			"f": {
				Type:     "db.find",
				Services: map[string]string{"database": "db"},
				Config: map[string]any{
					"table": "users_create",
					"where": map[string]any{"email": "alice@example.com"},
				},
			},
		},
	}
	execCtx2 := runNode(t, svcReg, nodeReg, findWF, nil)
	fout, ok := execCtx2.GetOutput("f")
	require.True(t, ok)
	// db.find returns []map[string]any
	rows := fout.([]map[string]any)
	require.Len(t, rows, 1)
	assert.Equal(t, "Alice", rows[0]["name"])
}

func TestDBUpdateCountDelete_Engine(t *testing.T) {
	svcReg, nodeReg, gdb := setupDB(t, "users_mut")
	require.NoError(t, gdb.Exec(
		"INSERT INTO users_mut (name, email) VALUES ('Bob','bob@example.com')").Error)

	updateWF := engine.WorkflowConfig{
		ID: "db-update",
		Nodes: map[string]engine.NodeConfig{
			"u": {
				Type:     "db.update",
				Services: map[string]string{"database": "db"},
				Config: map[string]any{
					"table": "users_mut",
					"data":  map[string]any{"name": "Bobby"},
					"where": map[string]any{"email": "bob@example.com"},
				},
			},
		},
	}
	uctx := runNode(t, svcReg, nodeReg, updateWF, nil)
	uout, ok := uctx.GetOutput("u")
	require.True(t, ok)
	// db.update returns map[string]any{"rows_affected": int64}
	updateMap := uout.(map[string]any)
	assert.EqualValues(t, int64(1), updateMap["rows_affected"])

	var name string
	require.NoError(t, gdb.Table("users_mut").Select("name").Where("email = ?", "bob@example.com").Scan(&name).Error)
	assert.Equal(t, "Bobby", name)

	countWF := engine.WorkflowConfig{
		ID: "db-count",
		Nodes: map[string]engine.NodeConfig{
			"n": {
				Type:     "db.count",
				Services: map[string]string{"database": "db"},
				Config:   map[string]any{"table": "users_mut"},
			},
		},
	}
	cctx := runNode(t, svcReg, nodeReg, countWF, nil)
	cout, ok := cctx.GetOutput("n")
	require.True(t, ok)
	// db.count returns map[string]any{"count": int64}
	countMap := cout.(map[string]any)
	assert.EqualValues(t, int64(1), countMap["count"])

	deleteWF := engine.WorkflowConfig{
		ID: "db-delete",
		Nodes: map[string]engine.NodeConfig{
			"d": {
				Type:     "db.delete",
				Services: map[string]string{"database": "db"},
				Config: map[string]any{
					"table": "users_mut",
					"where": map[string]any{"email": "bob@example.com"},
				},
			},
		},
	}
	runNode(t, svcReg, nodeReg, deleteWF, nil)
	var remaining int64
	require.NoError(t, gdb.Table("users_mut").Count(&remaining).Error)
	assert.Equal(t, int64(0), remaining)
}

func TestDBUpsertAndFindOne_Engine(t *testing.T) {
	svcReg, nodeReg, gdb := setupDB(t, "users_up")

	upsertWF := engine.WorkflowConfig{
		ID: "db-upsert",
		Nodes: map[string]engine.NodeConfig{
			"u": {
				Type:     "db.upsert",
				Services: map[string]string{"database": "db"},
				Config: map[string]any{
					"table":    "users_up",
					"data":     map[string]any{"name": "Cara", "email": "cara@example.com"},
					"conflict": "email",
				},
			},
		},
	}
	runNode(t, svcReg, nodeReg, upsertWF, nil)
	// Second upsert with same conflict key updates rather than duplicating.
	upsertWF.Nodes["u"] = engine.NodeConfig{
		Type:     "db.upsert",
		Services: map[string]string{"database": "db"},
		Config: map[string]any{
			"table":    "users_up",
			"data":     map[string]any{"name": "Cara2", "email": "cara@example.com"},
			"conflict": "email",
		},
	}
	runNode(t, svcReg, nodeReg, upsertWF, nil)

	var count int64
	require.NoError(t, gdb.Table("users_up").Count(&count).Error)
	assert.Equal(t, int64(1), count)

	findOneWF := engine.WorkflowConfig{
		ID: "db-findone",
		Nodes: map[string]engine.NodeConfig{
			"f": {
				Type:     "db.findOne",
				Services: map[string]string{"database": "db"},
				Config: map[string]any{
					"table": "users_up",
					"where": map[string]any{"email": "cara@example.com"},
				},
			},
		},
	}
	fctx := runNode(t, svcReg, nodeReg, findOneWF, nil)
	fout, ok := fctx.GetOutput("f")
	require.True(t, ok)
	assert.Equal(t, "Cara2", fout.(map[string]any)["name"])
}

func TestDBFindOne_NotFound_Engine(t *testing.T) {
	svcReg, nodeReg, _ := setupDB(t, "users_nf")

	// required defaults to true → NotFoundError surfaces as workflow error.
	wf := engine.WorkflowConfig{
		ID: "db-findone-notfound",
		Nodes: map[string]engine.NodeConfig{
			"f": {
				Type:     "db.findOne",
				Services: map[string]string{"database": "db"},
				Config: map[string]any{
					"table": "users_nf",
					"where": map[string]any{"email": "nobody@example.com"},
				},
			},
		},
	}
	graph, err := engine.Compile(wf, nodeReg)
	require.NoError(t, err)
	execCtx := engine.NewExecutionContext(engine.WithInput(nil))
	err = engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.Error(t, err) // not-found with required:true → workflow fails

	// required:false → success output with nil value.
	wfOpt := engine.WorkflowConfig{
		ID: "db-findone-notfound-optional",
		Nodes: map[string]engine.NodeConfig{
			"f": {
				Type:     "db.findOne",
				Services: map[string]string{"database": "db"},
				Config: map[string]any{
					"table":    "users_nf",
					"where":    map[string]any{"email": "nobody@example.com"},
					"required": false,
				},
			},
		},
	}
	fctx := runNode(t, svcReg, nodeReg, wfOpt, nil)
	fout, ok := fctx.GetOutput("f")
	require.True(t, ok)
	assert.Nil(t, fout) // required:false → nil when no row matches
}

func TestCreate_JSONBArrayRoundTrip(t *testing.T) {
	url := containers.StartPostgres(t)
	svc, err := (&Plugin{}).CreateService(map[string]any{"driver": "postgres", "url": url})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	db := svc.(*gorm.DB)

	if err := db.Exec(`DROP TABLE IF EXISTS jsonb_edits`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE jsonb_edits (id serial primary key, operations jsonb)`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Exec(`DROP TABLE IF EXISTS jsonb_edits`).Error })

	ops := []any{
		map[string]any{"type": "insert", "content": "A", "position": 0},
		map[string]any{"type": "delete", "length": 2, "position": 5},
	}
	row, err := marshalJSONComposites(map[string]any{"operations": ops})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := db.Table("jsonb_edits").Create(row).Error; err != nil {
		t.Fatalf("multi-element array insert failed (the #237 bug): %v", err)
	}

	var got string
	if err := db.Raw(`SELECT operations::text FROM jsonb_edits ORDER BY id DESC LIMIT 1`).Scan(&got).Error; err != nil {
		t.Fatal(err)
	}
	// Stored as a JSON array, not a bare object or record tuple.
	if got == "" || got[0] != '[' {
		t.Errorf("expected JSON array in jsonb column, got %q", got)
	}
}

// TestDBCreate_JSONBColumnStructuredOutput_Engine verifies that db.create returns
// composite (JSONB) columns as structured Go values (slice/map), not raw []byte
// that would serialize as base64 in JSON output (symmetric with db.upsert).
func TestDBCreate_JSONBColumnStructuredOutput_Engine(t *testing.T) {
	url := containers.StartPostgres(t)
	svc, err := (&Plugin{}).CreateService(map[string]any{"driver": "postgres", "url": url})
	require.NoError(t, err)
	gdb := svc.(*gorm.DB)
	require.NoError(t, gdb.Exec(`CREATE TABLE edits_exec (id serial PRIMARY KEY, operations jsonb)`).Error)
	t.Cleanup(func() { gdb.Exec("DROP TABLE IF EXISTS edits_exec") }) //nolint:errcheck

	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("db", svc, nil))
	nodeReg := registry.NewNodeRegistry()
	require.NoError(t, nodeReg.RegisterFromPlugin(&Plugin{}))

	ops := []any{
		map[string]any{"type": "insert", "content": "A", "position": 0},
		map[string]any{"type": "delete", "length": 2, "position": 5},
	}

	wf := engine.WorkflowConfig{
		ID: "db-create-jsonb",
		Nodes: map[string]engine.NodeConfig{
			"c": {
				Type:     "db.create",
				Services: map[string]string{"database": "db"},
				Config: map[string]any{
					"table": "edits_exec",
					"data":  map[string]any{"operations": ops},
				},
			},
		},
	}

	execCtx := runNode(t, svcReg, nodeReg, wf, nil)
	out, ok := execCtx.GetOutput("c")
	require.True(t, ok)
	row := out.(map[string]any)

	opsOut := row["operations"]
	require.NotNil(t, opsOut, "operations must be present in output")

	_, isBytes := opsOut.([]byte)
	assert.False(t, isBytes, "operations must NOT be []byte (base64 in JSON), got %T: %v", opsOut, opsOut)

	_, isString := opsOut.(string)
	assert.False(t, isString, "operations must NOT be a bare string blob, got %T: %v", opsOut, opsOut)

	// Must be re-marshallable as a JSON array (structured slice).
	b, err := json.Marshal(opsOut)
	require.NoError(t, err)
	assert.Equal(t, byte('['), b[0], "JSON of operations must start with '[', got %s", b)
}

func TestDBCreate_MissingTable_Engine(t *testing.T) {
	url := containers.StartPostgres(t)
	svc, err := (&Plugin{}).CreateService(map[string]any{"driver": "postgres", "url": url})
	require.NoError(t, err)
	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("db", svc, nil))
	nodeReg := registry.NewNodeRegistry()
	require.NoError(t, nodeReg.RegisterFromPlugin(&Plugin{}))

	wf := engine.WorkflowConfig{
		ID: "db-err",
		Nodes: map[string]engine.NodeConfig{
			"c": {
				Type:     "db.create",
				Services: map[string]string{"database": "db"},
				Config: map[string]any{
					"table": "does_not_exist",
					"data":  map[string]any{"name": "X"},
				},
			},
		},
	}
	graph, err := engine.Compile(wf, nodeReg)
	require.NoError(t, err)
	execCtx := engine.NewExecutionContext(engine.WithInput(nil))
	err = engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.Error(t, err) // unknown table → workflow fails, no panic
}
