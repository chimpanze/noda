package auth

import (
	"testing"
)

// TestNodeDescriptorContract verifies the descriptor contract for every
// registered node. The editor consumes these descriptors to render node
// palettes, config forms, and output ports, so each field must be populated
// and consistent with the executor the factory produces.
func TestNodeDescriptorContract(t *testing.T) {
	regs := (&Plugin{}).Nodes()
	if len(regs) != 8 {
		t.Fatalf("expected 8 node registrations, got %d", len(regs))
	}

	seen := make(map[string]bool, len(regs))
	for _, reg := range regs {
		d := reg.Descriptor
		name := d.Name()
		t.Run(name, func(t *testing.T) {
			if name == "" {
				t.Fatal("Name() must be non-empty")
			}
			if seen[name] {
				t.Fatalf("duplicate node name %q", name)
			}
			seen[name] = true

			if d.Description() == "" {
				t.Fatal("Description() must be non-empty")
			}

			deps := d.ServiceDeps()
			auth, ok := deps["auth"]
			if !ok || auth.Prefix != "auth" || !auth.Required {
				t.Fatalf("ServiceDeps() must contain required slot %q with prefix %q, got %+v", "auth", "auth", deps)
			}
			database, ok := deps["database"]
			if !ok || database.Prefix != "db" || !database.Required {
				t.Fatalf("ServiceDeps() must contain required slot %q with prefix %q, got %+v", "database", "db", deps)
			}

			schema := d.ConfigSchema()
			if schema["type"] != "object" {
				t.Fatalf("ConfigSchema() type must be %q, got %v", "object", schema["type"])
			}
			props, ok := schema["properties"].(map[string]any)
			if !ok || props == nil {
				t.Fatalf("ConfigSchema() must have a non-nil properties map, got %T", schema["properties"])
			}

			exec := reg.Factory(nil)
			if exec == nil {
				t.Fatal("Factory(nil) must return an executor")
			}
			outputs := exec.Outputs()
			if len(outputs) == 0 {
				t.Fatal("Outputs() must be non-empty")
			}
			descs := d.OutputDescriptions()
			for _, out := range outputs {
				if _, ok := descs[out]; !ok {
					t.Errorf("output %q returned by Outputs() has no entry in OutputDescriptions()", out)
				}
			}
			for out := range descs {
				found := false
				for _, o := range outputs {
					if o == out {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("OutputDescriptions() entry %q is not returned by Outputs()", out)
				}
			}
		})
	}
}
