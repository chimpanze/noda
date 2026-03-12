package registry

import (
	"context"
	"fmt"
	"testing"

	"github.com/chimpanze/noda/pkg/api"
)

type benchDescriptor struct {
	name string
}

func (d *benchDescriptor) Name() string                           { return d.name }
func (d *benchDescriptor) Description() string                    { return "" }
func (d *benchDescriptor) ServiceDeps() map[string]api.ServiceDep { return nil }
func (d *benchDescriptor) ConfigSchema() map[string]any           { return nil }

type benchExecutor struct{}

func (e *benchExecutor) Outputs() []string { return []string{"success", "error"} }
func (e *benchExecutor) Execute(_ context.Context, _ api.ExecutionContext, _ map[string]any, _ map[string]any) (string, any, error) {
	return "success", nil, nil
}

type benchPlugin struct {
	name   string
	prefix string
	nodes  []api.NodeRegistration
}

func (p *benchPlugin) Name() string                                     { return p.name }
func (p *benchPlugin) Prefix() string                                   { return p.prefix }
func (p *benchPlugin) Nodes() []api.NodeRegistration                    { return p.nodes }
func (p *benchPlugin) HasServices() bool                                { return false }
func (p *benchPlugin) CreateService(config map[string]any) (any, error) { return nil, nil }
func (p *benchPlugin) HealthCheck(service any) error                    { return nil }
func (p *benchPlugin) Shutdown(service any) error                       { return nil }

func setupBenchNodeRegistry(b *testing.B, count int) *NodeRegistry {
	b.Helper()
	reg := NewNodeRegistry()
	var regs []api.NodeRegistration
	for i := 0; i < count; i++ {
		regs = append(regs, api.NodeRegistration{
			Descriptor: &benchDescriptor{name: fmt.Sprintf("node%d", i)},
			Factory:    func(map[string]any) api.NodeExecutor { return &benchExecutor{} },
		})
	}
	p := &benchPlugin{name: "bench", prefix: "bench", nodes: regs}
	if err := reg.RegisterFromPlugin(p); err != nil {
		b.Fatal(err)
	}
	return reg
}

func BenchmarkNodeRegistry_GetFactory(b *testing.B) {
	reg := setupBenchNodeRegistry(b, 50)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		reg.GetFactory("bench.node25")
	}
}

func BenchmarkNodeRegistry_GetFactory_Parallel(b *testing.B) {
	reg := setupBenchNodeRegistry(b, 50)
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			reg.GetFactory("bench.node25")
		}
	})
}

func BenchmarkNodeRegistry_GetDescriptor(b *testing.B) {
	reg := setupBenchNodeRegistry(b, 50)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		reg.GetDescriptor("bench.node25")
	}
}

func BenchmarkNodeRegistry_AllTypes(b *testing.B) {
	reg := setupBenchNodeRegistry(b, 50)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		reg.AllTypes()
	}
}

func BenchmarkServiceRegistry_Get(b *testing.B) {
	reg := NewServiceRegistry()
	p := &benchPlugin{name: "bench", prefix: "bench"}
	for i := 0; i < 20; i++ {
		_ = reg.Register(fmt.Sprintf("svc_%d", i), struct{}{}, p)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		reg.Get("svc_10")
	}
}

func BenchmarkServiceRegistry_Get_Parallel(b *testing.B) {
	reg := NewServiceRegistry()
	p := &benchPlugin{name: "bench", prefix: "bench"}
	for i := 0; i < 20; i++ {
		_ = reg.Register(fmt.Sprintf("svc_%d", i), struct{}{}, p)
	}
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			reg.Get("svc_10")
		}
	})
}
