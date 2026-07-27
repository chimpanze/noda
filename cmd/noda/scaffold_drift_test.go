package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/chimpanze/noda/internal/scaffold"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readScaffoldTree returns every regular file under root, keyed by
// root-relative path.
func readScaffoldTree(t *testing.T, root string) map[string]string {
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

// #449: `noda init` and the MCP noda_scaffold_project tool were two
// hand-maintained copies of the same project and had already diverged. Both
// now render internal/scaffold's templates; this pins `noda init` to that
// shared source, and TestScaffoldProjectHandler_MatchesSharedScaffold in
// internal/mcp pins the other side. Together they mean a file added to one
// surface but not the templates reddens.
func TestScaffoldProject_MatchesSharedScaffold(t *testing.T) {
	tmpDir := t.TempDir()
	viaCLI := filepath.Join(tmpDir, "app")
	viaShared := filepath.Join(tmpDir, "app") // same base name: CLAUDE.md/README.md interpolate it

	require.NoError(t, scaffoldProject(viaCLI, false))
	cliTree := readScaffoldTree(t, viaCLI)

	require.NoError(t, os.RemoveAll(viaShared))
	_, err := scaffold.WriteProject(viaShared, false)
	require.NoError(t, err)
	sharedTree := readScaffoldTree(t, viaShared)

	// Guard the fixture: equality between two empty trees proves nothing.
	require.Contains(t, cliTree, "noda.json")
	require.Greater(t, len(cliTree), 8, "scaffold should write the full project, got %d files", len(cliTree))

	// .env carries a freshly generated secret, so it differs by design.
	assert.NotEqual(t, cliTree[".env"], sharedTree[".env"], "each scaffold generates its own JWT_SECRET")
	delete(cliTree, ".env")
	delete(sharedTree, ".env")

	assert.Equal(t, sharedTree, cliTree,
		"`noda init` must write exactly internal/scaffold's templates, with no files added, dropped, or edited here")
}
