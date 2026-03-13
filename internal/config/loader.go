package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// RawConfig holds the raw parsed JSON from all config files.
type RawConfig struct {
	Root        map[string]any
	Overlay     map[string]any            // nil if no overlay
	Vars        map[string]string         // from vars.json (optional)
	Schemas     map[string]map[string]any // keyed by file path
	Routes      map[string]map[string]any
	Workflows   map[string]map[string]any
	Workers     map[string]map[string]any
	Schedules   map[string]map[string]any
	Connections map[string]map[string]any
	Tests       map[string]map[string]any
	Models      map[string]map[string]any
}

// LoadAll loads all discovered JSON files into a RawConfig, collecting all errors.
func LoadAll(discovered *DiscoveredFiles) (*RawConfig, []error) {
	var errs []error
	rc := &RawConfig{
		Schemas:     make(map[string]map[string]any),
		Routes:      make(map[string]map[string]any),
		Workflows:   make(map[string]map[string]any),
		Workers:     make(map[string]map[string]any),
		Schedules:   make(map[string]map[string]any),
		Connections: make(map[string]map[string]any),
		Tests:       make(map[string]map[string]any),
		Models:      make(map[string]map[string]any),
	}

	// Load root (required)
	root, err := loadJSONFile(discovered.Root)
	if err != nil {
		errs = append(errs, err)
	} else {
		rc.Root = root
	}

	// Load overlay (optional)
	if discovered.Overlay != "" {
		overlay, err := loadJSONFile(discovered.Overlay)
		if err != nil {
			errs = append(errs, err)
		} else {
			rc.Overlay = overlay
		}
	}

	// Load vars.json (optional)
	if discovered.Vars != "" {
		varsRaw, err := loadJSONFile(discovered.Vars)
		if err != nil {
			errs = append(errs, err)
		} else {
			vars := make(map[string]string, len(varsRaw))
			for k, v := range varsRaw {
				s, ok := v.(string)
				if !ok {
					errs = append(errs, fmt.Errorf("%s: value for %q must be a string", discovered.Vars, k))
					continue
				}
				vars[k] = s
			}
			rc.Vars = vars
		}
	}

	// Load categorized files
	categories := []struct {
		paths  []string
		target map[string]map[string]any
	}{
		{discovered.Schemas, rc.Schemas},
		{discovered.Routes, rc.Routes},
		{discovered.Workflows, rc.Workflows},
		{discovered.Workers, rc.Workers},
		{discovered.Schedules, rc.Schedules},
		{discovered.Connections, rc.Connections},
		{discovered.Tests, rc.Tests},
		{discovered.Models, rc.Models},
	}

	for _, cat := range categories {
		for _, path := range cat.paths {
			data, err := loadJSONFile(path)
			if err != nil {
				errs = append(errs, err)
			} else {
				cat.target[path] = data
			}
		}
	}

	if len(errs) > 0 {
		return rc, errs
	}
	return rc, nil
}

// loadJSONFile reads and parses a single JSON file.
func loadJSONFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	// Strip BOM if present
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		data = data[3:]
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		if syntaxErr, ok := err.(*json.SyntaxError); ok {
			return nil, fmt.Errorf("%s: invalid JSON at offset %d: %s", path, syntaxErr.Offset, syntaxErr.Error())
		}
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	return result, nil
}
