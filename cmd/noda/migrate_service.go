package main

import (
	"fmt"
	"sort"
	"strings"
)

// resolveMigrateService picks the database service for `noda migrate`.
//
// When the --service flag was not explicitly set (explicit == false) and the
// config contains exactly one Postgres service, that service is used — so the
// shipped examples (which name it "main-db") work without the flag. When the
// flag is explicit, or auto-detection is ambiguous/empty, the requested name
// must exist; otherwise an error lists the available service names.
func resolveMigrateService(services map[string]any, flag string, explicit bool) (string, error) {
	if !explicit {
		if pg := postgresServiceNames(services); len(pg) == 1 {
			return pg[0], nil
		} else if len(pg) > 1 {
			return "", fmt.Errorf("multiple postgres services found (%s); specify one with --service", strings.Join(pg, ", "))
		}
	}
	if _, ok := services[flag].(map[string]any); ok {
		return flag, nil
	}
	return "", fmt.Errorf("service %q not found in config (available: %s)", flag, strings.Join(serviceNames(services), ", "))
}

func postgresServiceNames(services map[string]any) []string {
	var names []string
	for name, raw := range services {
		if m, ok := raw.(map[string]any); ok && m["plugin"] == "postgres" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func serviceNames(services map[string]any) []string {
	names := make([]string, 0, len(services))
	for name := range services {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
