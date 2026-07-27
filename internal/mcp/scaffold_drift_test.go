package mcp

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/chimpanze/noda/internal/scaffold"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readTree returns every regular file under root, keyed by root-relative path.
func readTree(t *testing.T, root string) map[string]string {
	t.Helper()
	fsys := os.DirFS(root)
	files := map[string]string{}
	require.NoError(t, fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, readErr := fs.ReadFile(fsys, path)
		if readErr != nil {
			return readErr
		}
		files[path] = string(data)
		return nil
	}))
	return files
}

// #449: the MCP scaffold and `noda init` were two hand-maintained copies of the
// same project and had already diverged — MCP's noda.json was missing
// global_middleware, its sample test asserted the wrong key ("output" rather
// than "outputs"), and it shipped none of CLAUDE.md, README.md, .mcp.json, or
// .claude/settings.json. Both surfaces now render internal/scaffold's
// templates; this pins the MCP tool to that shared source so a bespoke file
// list cannot be reintroduced here without reddening.
//
// `noda init` is pinned to the same source from its own side by
// TestScaffoldProject_MatchesSharedScaffold in cmd/noda, which cannot live
// here because cmd/noda is package main.
func TestScaffoldProjectHandler_MatchesSharedScaffold(t *testing.T) {
	tmpDir := t.TempDir()
	viaMCP := filepath.Join(tmpDir, "via-mcp")
	viaShared := filepath.Join(tmpDir, "via-mcp") // same base name: CLAUDE.md/README.md interpolate it

	req := makeCallToolRequest("noda_scaffold_project", map[string]any{"path": viaMCP})
	result, err := scaffoldProjectHandler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)
	mcpTree := readTree(t, viaMCP)

	require.NoError(t, os.RemoveAll(viaShared))
	_, err = scaffold.WriteProject(viaShared, false)
	require.NoError(t, err)
	sharedTree := readTree(t, viaShared)

	// Guard the fixture: an empty or near-empty tree would make equality
	// meaningless, and the drift that motivated this test was in these files.
	for _, required := range []string{"noda.json", "CLAUDE.md", "README.md", ".mcp.json", ".claude/settings.json", "tests/hello.test.json"} {
		require.Contains(t, mcpTree, required, "MCP scaffold must write %s", required)
	}
	assert.Contains(t, mcpTree["noda.json"], "global_middleware",
		"the concrete drift #449 reported: MCP's noda.json lacked the recover/requestid/logger stack")
	assert.Contains(t, mcpTree["tests/hello.test.json"], `"outputs"`,
		"expect.outputs is the real key; MCP's copy had drifted to \"output\"")

	// .env carries a freshly generated secret, so it differs by design.
	assert.NotEqual(t, mcpTree[".env"], sharedTree[".env"], "each scaffold generates its own JWT_SECRET")
	delete(mcpTree, ".env")
	delete(sharedTree, ".env")

	assert.Equal(t, sharedTree, mcpTree,
		"the MCP scaffold must be exactly internal/scaffold's templates, with no files added, dropped, or edited here")
}
