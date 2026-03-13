package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DiscoveredFiles holds categorized config file paths found in a project directory.
type DiscoveredFiles struct {
	Root        string   // path to noda.json (required)
	Overlay     string   // path to noda.{env}.json (optional, empty if not found)
	Vars        string   // path to vars.json (optional, empty if not found)
	Schemas     []string // schemas/**/*.json
	Routes      []string // routes/**/*.json
	Workflows   []string // workflows/**/*.json
	Workers     []string // workers/**/*.json
	Schedules   []string // schedules/**/*.json
	Connections []string // connections/**/*.json
	Tests       []string // tests/**/*.json
	Models      []string // models/**/*.json
}

// Discover scans rootPath for config files and categorizes them by convention.
// The env parameter determines which overlay file to look for (e.g., "development" → noda.development.json).
func Discover(rootPath string, env string) (*DiscoveredFiles, error) {
	rootFile := filepath.Join(rootPath, "noda.json")
	if _, err := os.Stat(rootFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("missing required config file: %s", rootFile)
	}

	d := &DiscoveredFiles{
		Root: rootFile,
	}

	// Check for overlay file
	if env != "" {
		overlayFile := filepath.Join(rootPath, fmt.Sprintf("noda.%s.json", env))
		if _, err := os.Stat(overlayFile); err == nil {
			d.Overlay = overlayFile
		}
	}

	// Check for vars.json
	varsFile := filepath.Join(rootPath, "vars.json")
	if _, err := os.Stat(varsFile); err == nil {
		d.Vars = varsFile
	}

	// Scan each config directory
	dirs := []struct {
		name   string
		target *[]string
	}{
		{"schemas", &d.Schemas},
		{"routes", &d.Routes},
		{"workflows", &d.Workflows},
		{"workers", &d.Workers},
		{"schedules", &d.Schedules},
		{"connections", &d.Connections},
		{"tests", &d.Tests},
		{"models", &d.Models},
	}

	for _, dir := range dirs {
		dirPath := filepath.Join(rootPath, dir.name)
		if _, err := os.Stat(dirPath); os.IsNotExist(err) {
			continue
		}
		files, err := scanJSONFiles(dirPath)
		if err != nil {
			return nil, fmt.Errorf("scanning %s: %w", dir.name, err)
		}
		*dir.target = files
	}

	return d, nil
}

// scanJSONFiles recursively finds all .json files in a directory.
func scanJSONFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.ToLower(filepath.Ext(path)) == ".json" {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}
