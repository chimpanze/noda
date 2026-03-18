package pathutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRoot_Dot(t *testing.T) {
	root, err := NewRoot(".")
	require.NoError(t, err)

	wd, _ := os.Getwd()
	assert.Equal(t, wd, root.String())
}

func TestNewRoot_Relative(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	require.NoError(t, os.Mkdir(sub, 0o755))

	root, err := NewRoot(sub)
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(root.String()))
	assert.Equal(t, sub, root.String())
}

func TestContains_ExactMatch(t *testing.T) {
	root, err := NewRoot(t.TempDir())
	require.NoError(t, err)

	assert.True(t, root.Contains(root.String()))
}

func TestContains_ValidSubpath(t *testing.T) {
	root, err := NewRoot(t.TempDir())
	require.NoError(t, err)

	assert.True(t, root.Contains(filepath.Join(root.String(), "sub", "file.json")))
}

func TestContains_SiblingAttack(t *testing.T) {
	// /home/noda should NOT contain /home/noda-evil
	dir := t.TempDir()
	root, err := NewRoot(dir)
	require.NoError(t, err)

	assert.False(t, root.Contains(dir+"-evil"))
}

func TestContains_DotDotTraversal(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	require.NoError(t, os.Mkdir(sub, 0o755))

	root, err := NewRoot(sub)
	require.NoError(t, err)

	// ../sibling resolves to dir, which is outside sub
	assert.False(t, root.Contains(filepath.Join(sub, "..", "sibling")))
}

func TestContains_Parent(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	require.NoError(t, os.Mkdir(sub, 0o755))

	root, err := NewRoot(sub)
	require.NoError(t, err)

	assert.False(t, root.Contains(dir))
}

func TestResolve_CleanRelative(t *testing.T) {
	root, err := NewRoot(t.TempDir())
	require.NoError(t, err)

	abs, err := root.Resolve("routes/api.json")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root.String(), "routes", "api.json"), abs)
}

func TestResolve_AbsoluteInput(t *testing.T) {
	root, err := NewRoot(t.TempDir())
	require.NoError(t, err)

	_, err = root.Resolve("/etc/passwd")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "absolute")
}

func TestResolve_DotDotEscape(t *testing.T) {
	root, err := NewRoot(t.TempDir())
	require.NoError(t, err)

	_, err = root.Resolve("../../etc/passwd")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "outside root")
}

func TestResolve_DotDotInMiddle(t *testing.T) {
	root, err := NewRoot(t.TempDir())
	require.NoError(t, err)

	// routes/../routes/api.json should resolve fine (stays within root)
	abs, err := root.Resolve("routes/../routes/api.json")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root.String(), "routes", "api.json"), abs)
}

func TestRel_RoundTrip(t *testing.T) {
	root, err := NewRoot(t.TempDir())
	require.NoError(t, err)

	rel := "routes/api.json"
	abs, err := root.Resolve(rel)
	require.NoError(t, err)

	got := root.Rel(abs)
	assert.Equal(t, rel, got)
}

func TestRel_Empty(t *testing.T) {
	root, err := NewRoot(t.TempDir())
	require.NoError(t, err)

	assert.Equal(t, "", root.Rel(""))
}

func TestJoin(t *testing.T) {
	root, err := NewRoot(t.TempDir())
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(root.String(), "models"), root.Join("models"))
	assert.Equal(t, filepath.Join(root.String(), "a", "b"), root.Join("a", "b"))
}
