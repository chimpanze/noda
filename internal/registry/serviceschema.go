package registry

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	json "github.com/goccy/go-json"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// serviceSchemaCache caches compiled service-config schemas keyed by plugin
// name plus a content hash of the schema itself, mirroring internal/config/
// validator.go's schemaCache: schemas are compiled once and reused across
// every dry-run validation call (validate/boot/editor/MCP/hot-reload all
// funnel through ValidateStartupDryRun).
//
// The content hash matters because plugin name alone is not a stable cache
// key across process lifetimes within the same test binary: registry's own
// tests register throwaway plugins named "auth" with different (often
// minimal/incomplete) ServiceConfigSchemas to exercise validation edge cases.
// Without the hash, whichever test ran first "wins" the cache slot for
// "auth" and every later caller — including the real auth plugin's own
// service_schema_audit_test.go — silently gets that stale compiled schema
// back instead of its own, weakening the audit's compile gate without any
// visible error.
var (
	serviceSchemaCacheMu sync.RWMutex
	serviceSchemaCache   = make(map[string]*jsonschema.Schema)
)

// serviceSchemaCacheKey combines the plugin name with a hash of the
// already-marshaled schema JSON so that two different schemas registered
// under the same plugin name never collide in the cache.
func serviceSchemaCacheKey(pluginName string, raw []byte) string {
	sum := sha256.Sum256(raw)
	// Not "#": the resource name is also used verbatim as the compiled
	// schema's resource URI (below), and jsonschema treats a "#" suffix as a
	// URI fragment/anchor reference rather than part of the resource name.
	return pluginName + "@" + hex.EncodeToString(sum[:])
}

// compileServiceSchema compiles a plugin's ServiceConfigSchema (a
// map[string]any authored the same way as NodeDescriptor.ConfigSchema) into a
// *jsonschema.Schema using the real santhosh-tekuri/jsonschema library —
// unlike node ConfigSchemas, service configs are validated with the real
// library rather than the custom ValidateNodeConfig walker, because service
// config values never contain "{{ }}" runtime expressions (a service is
// constructed once at boot, not evaluated per-request) so the walker's
// expression-bypass and permissive-oneOf semantics aren't needed here.
//
// The schema map is round-tripped through JSON (marshal then unmarshal into
// `any`) before being handed to the compiler, so the compiler sees JSON-native
// types (float64, map[string]any, []any) instead of whatever Go-literal types
// the plugin author wrote the schema map with — the same reason
// internal/config/validator.go's getCompiledSchema reads its embedded schemas
// as raw JSON bytes rather than passing Go structs to the compiler directly.
func compileServiceSchema(pluginName string, schema map[string]any) (*jsonschema.Schema, error) {
	raw, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("marshal service schema: %w", err)
	}

	cacheKey := serviceSchemaCacheKey(pluginName, raw)

	serviceSchemaCacheMu.RLock()
	if s, ok := serviceSchemaCache[cacheKey]; ok {
		serviceSchemaCacheMu.RUnlock()
		return s, nil
	}
	serviceSchemaCacheMu.RUnlock()

	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("unmarshal service schema: %w", err)
	}

	resourceName := "service-schema://" + cacheKey
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(resourceName, doc); err != nil {
		return nil, fmt.Errorf("add service schema resource: %w", err)
	}
	compiled, err := compiler.Compile(resourceName)
	if err != nil {
		return nil, fmt.Errorf("compile service schema: %w", err)
	}

	serviceSchemaCacheMu.Lock()
	serviceSchemaCache[cacheKey] = compiled
	serviceSchemaCacheMu.Unlock()

	return compiled, nil
}

// CompileServiceSchema exposes compileServiceSchema to tests outside package
// registry. service_schema_audit_test.go lives in an external (_test)
// package — package registry_test — because it needs to import every plugin
// package, which themselves import internal/registry; giving that test file
// package-level access to compileServiceSchema would create an import cycle,
// so this thin exported wrapper is the seam instead. Production code (see
// ValidateStartupDryRun) calls compileServiceSchema directly.
func CompileServiceSchema(pluginName string, schema map[string]any) (*jsonschema.Schema, error) {
	return compileServiceSchema(pluginName, schema)
}

// validateAgainst validates a service config against its compiled schema and
// flattens any resulting *jsonschema.ValidationError tree into a single
// semicolon-joined message, mirroring internal/config/validator.go's
// collectLeafErrors tree-walk (that function keeps a []ValidationError per
// leaf for structured FilePath/JSONPath reporting; this one only needs a
// human-readable summary for a single fmt.Errorf call site, so it flattens
// straight to strings).
//
// $env() policy (#376): by the time ValidateStartupDryRun runs, the config
// pipeline has already resolved every $env() reference, so service config
// values arriving here are plain strings/numbers/bools — never an unresolved
// "$env(...)" literal. No special-casing is needed; the schemas declared by
// each plugin's ServiceConfigSchema (Tasks 1+2) already type ToInt-parsed
// fields as accepting string OR integer to match the post-resolution shape.
func validateAgainst(schema *jsonschema.Schema, config map[string]any) error {
	err := schema.Validate(config)
	if err == nil {
		return nil
	}
	ve, ok := err.(*jsonschema.ValidationError)
	if !ok {
		return err
	}
	var msgs []string
	flattenValidationError(ve, &msgs)
	return fmt.Errorf("%s", strings.Join(msgs, "; "))
}

func flattenValidationError(ve *jsonschema.ValidationError, msgs *[]string) {
	if len(ve.Causes) == 0 {
		loc := strings.Join(ve.InstanceLocation, ".")
		if loc == "" {
			*msgs = append(*msgs, ve.Error())
			return
		}
		*msgs = append(*msgs, fmt.Sprintf("%s: %s", loc, ve.Error()))
		return
	}
	for _, cause := range ve.Causes {
		flattenValidationError(cause, msgs)
	}
}
