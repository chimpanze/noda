package scaffold

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
)

// templates/* excludes dotfiles; list them explicitly.
//
//go:embed templates/* templates/.env.example templates/.mcp.json templates/.claude/settings.json
var templateFS embed.FS

// templateRoot is the directory prefix stripped from embedded paths to get a
// project-relative output path.
const templateRoot = "templates"

// envExamplePath is the committable template whose JWT_SECRET placeholder is
// replaced with a generated secret when writing .env.
const envExamplePath = ".env.example"

// projectData is the value .tmpl files are rendered against. Templates use
// [[ ]] delimiters so JSON and Go-template syntax do not collide.
type projectData struct {
	ProjectName string
}

// ConflictError reports scaffold target paths that already exist. Paths are
// project-relative and sorted; callers render them however suits their surface.
type ConflictError struct {
	Paths []string
}

func (e *ConflictError) Error() string {
	return "refusing to overwrite existing files: " + strings.Join(e.Paths, ", ")
}

// Files renders the project scaffold for a project named projectName and
// returns its contents keyed by project-relative path, with .tmpl suffixes
// already stripped. .env is not included: it carries a freshly generated
// secret and is produced by WriteProject.
//
// This is the single definition of what a Noda project scaffold contains.
// `noda init` and the MCP noda_scaffold_project tool both go through it, so
// the two entry points cannot produce different projects (#449).
func Files(projectName string) (map[string][]byte, error) {
	data := projectData{ProjectName: projectName}
	files := map[string][]byte{}

	err := fs.WalkDir(templateFS, templateRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		relPath := strings.TrimPrefix(path, templateRoot+"/")
		content, err := templateFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read template %s: %w", path, err)
		}

		if trimmed, isTemplate := strings.CutSuffix(relPath, ".tmpl"); isTemplate {
			relPath = trimmed
			tmpl, err := template.New(filepath.Base(path)).Delims("[[", "]]").Parse(string(content))
			if err != nil {
				return fmt.Errorf("parse template %s: %w", path, err)
			}
			var buf strings.Builder
			if err := tmpl.Execute(&buf, data); err != nil {
				return fmt.Errorf("execute template %s: %w", path, err)
			}
			content = []byte(buf.String())
		}

		files[relPath] = content
		return nil
	})
	if err != nil {
		return nil, err
	}

	return files, nil
}

// WriteProject scaffolds a project into dest, named after dest's base name.
// It returns the project-relative paths it created, sorted.
//
// Unless force is set, it fails with a *ConflictError before writing anything
// if any target already exists, so a refused scaffold never leaves a partial
// project behind.
func WriteProject(dest string, force bool) ([]string, error) {
	files, err := Files(filepath.Base(dest))
	if err != nil {
		return nil, err
	}

	// .env is generated rather than templated: a fresh project needs a
	// compliant JWT_SECRET (>=32 bytes) from the start (#381), while
	// .env.example stays committable with its placeholder.
	envExample, ok := files[envExamplePath]
	if !ok {
		return nil, fmt.Errorf("scaffold templates are missing %s", envExamplePath)
	}
	secret, err := GenerateJWTSecret()
	if err != nil {
		return nil, err
	}
	files[".env"] = []byte(ApplyJWTSecret(string(envExample), secret))

	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	if !force {
		var conflicts []string
		for _, path := range paths {
			if _, statErr := os.Stat(filepath.Join(dest, path)); statErr == nil {
				conflicts = append(conflicts, path)
			}
		}
		if len(conflicts) > 0 {
			return nil, &ConflictError{Paths: conflicts}
		}
	}

	for _, path := range paths {
		outPath := filepath.Join(dest, path)
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return nil, fmt.Errorf("create directory for %s: %w", path, err)
		}
		if err := os.WriteFile(outPath, files[path], 0644); err != nil {
			return nil, fmt.Errorf("write %s: %w", path, err)
		}
	}

	return paths, nil
}
